package service

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"time"
)

// FLV layout (Adobe FLV spec v10.1, annex E): a 9-byte header ("FLV",
// version, audio/video flags, header size), a 4-byte PreviousTagSize0 of 0,
// then tags. A tag is an 11-byte header (type, 24-bit data size, 24-bit
// timestamp in ms plus an 8-bit extension holding its upper bits, 24-bit
// stream id 0), its data, and a 4-byte PreviousTagSize of 11 + data size.
const (
	flvHeaderSize    = 9
	flvTagHeaderSize = 11
	flvTagAudio      = 8
	flvTagVideo      = 9
	flvTagScript     = 18
)

// flvSegmentGap is how many ms after the last tag of one file the first tag
// of the next is placed when files are joined.
const flvSegmentGap = 40

// errFLVDamaged marks an input joinFLV cut short or skipped.
var errFLVDamaged = errors.New("damaged FLV")

// flvJoin describes a file joinFLV wrote.
type flvJoin struct {
	tags     int
	size     int64
	duration time.Duration
	// damaged says which inputs were cut short at a truncated or malformed
	// tag, or skipped as not FLV at all.
	damaged []string
}

// joinFLV writes the FLV files inputs, in order, to out as one FLV:
//   - the first input's header, once;
//   - every input's audio and video tags, their sequence headers included
//     as the encoder settings may change between publish sessions, with the
//     timestamps of each later input shifted to continue flvSegmentGap ms
//     after the last tag written;
//   - the script data (onMetaData) the first input starts with, its
//     duration and filesize updated for the whole file; later inputs'
//     script data is dropped;
//   - PreviousTagSize fields computed afresh.
//
// An input ends at its first truncated or malformed tag, as SRS leaves one
// when it dies while writing; the tags before it are kept. An input that is
// not FLV is skipped. It fails when no input has audio or video.
func joinFLV(out *os.File, inputs []string) (flvJoin, error) {
	j := &flvJoiner{w: bufio.NewWriterSize(out, 1<<20), scriptInput: -1, durationAt: -1, filesizeAt: -1}
	var result flvJoin
	for i, input := range inputs {
		if err := j.addFile(i, input); err != nil {
			if !errors.Is(err, errFLVDamaged) {
				return result, err
			}
			result.damaged = append(result.damaged, fmt.Sprintf("%s: %v", input, err))
		}
	}
	if !j.media {
		return result, errors.New("recording has no audio or video")
	}
	if err := j.w.Flush(); err != nil {
		return result, err
	}
	duration := time.Duration(j.lastTS-j.firstTS) * time.Millisecond
	for _, field := range []struct {
		at    int64
		value float64
	}{{j.durationAt, duration.Seconds()}, {j.filesizeAt, float64(j.written)}} {
		if field.at < 0 {
			continue
		}
		var value [8]byte
		binary.BigEndian.PutUint64(value[:], math.Float64bits(field.value))
		if _, err := out.WriteAt(value[:], field.at); err != nil {
			return result, err
		}
	}
	result.tags, result.size, result.duration = j.tags, j.written, duration
	return result, nil
}

type flvJoiner struct {
	w       *bufio.Writer
	written int64
	header  bool // the FLV header is written
	tags    int
	media   bool // an audio or video tag is written
	// scriptInput is the input whose leading script data is kept.
	scriptInput     int
	firstTS, lastTS uint32
	// durationAt and filesizeAt locate the kept onMetaData's duration and
	// filesize values in the output, -1 when it has none.
	durationAt, filesizeAt int64
	buf                    []byte
}

func (j *flvJoiner) addFile(input int, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	r := bufio.NewReaderSize(file, 1<<20)

	var header [flvHeaderSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		if endOfData(err) {
			return fmt.Errorf("%w: no FLV header", errFLVDamaged)
		}
		return err
	}
	dataOffset := binary.BigEndian.Uint32(header[5:])
	if string(header[:3]) != "FLV" || dataOffset < flvHeaderSize {
		return fmt.Errorf("%w: not an FLV file", errFLVDamaged)
	}
	// The rest of the header, then PreviousTagSize0.
	if _, err := io.CopyN(io.Discard, r, int64(dataOffset-flvHeaderSize)+4); err != nil {
		if endOfData(err) {
			return nil // nothing was recorded
		}
		return err
	}
	if !j.header {
		binary.BigEndian.PutUint32(header[5:], flvHeaderSize)
		if err := j.write(header[:], []byte{0, 0, 0, 0}); err != nil {
			return err
		}
		j.header = true
	}

	first := true
	var shift int64
	var tag [flvTagHeaderSize]byte
	var trailer [4]byte
	for {
		if _, err := io.ReadFull(r, tag[:]); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			if endOfData(err) {
				return fmt.Errorf("%w: truncated tag header", errFLVDamaged)
			}
			return err
		}
		kind := tag[0]
		size := int(tag[1])<<16 | int(tag[2])<<8 | int(tag[3])
		if (kind != flvTagAudio && kind != flvTagVideo && kind != flvTagScript) || tag[8]|tag[9]|tag[10] != 0 {
			return fmt.Errorf("%w: malformed tag", errFLVDamaged)
		}
		data := j.buffer(size)
		if _, err := io.ReadFull(r, data); err != nil {
			if endOfData(err) {
				return fmt.Errorf("%w: truncated tag", errFLVDamaged)
			}
			return err
		}
		// A missing PreviousTagSize leaves the tag itself whole.
		_, trailerErr := io.ReadFull(r, trailer[:])
		if trailerErr != nil && !endOfData(trailerErr) {
			return trailerErr
		}
		if trailerErr == nil && binary.BigEndian.Uint32(trailer[:]) != uint32(flvTagHeaderSize+size) {
			return fmt.Errorf("%w: malformed tag", errFLVDamaged)
		}

		keep := kind != flvTagScript
		if !keep && !j.media && (j.tags == 0 || j.scriptInput == input) {
			keep, j.scriptInput = true, input
		}
		if keep {
			ts := flvTimestamp(tag)
			if first {
				first = false
				if j.tags > 0 {
					shift = int64(j.lastTS) + flvSegmentGap - int64(ts)
				}
			}
			if err := j.writeTag(tag, data, int64(ts)+shift); err != nil {
				return err
			}
		}
		if trailerErr != nil {
			return fmt.Errorf("%w: last tag's size field cut short", errFLVDamaged)
		}
	}
}

// writeTag writes a tag with timestamp ts and its PreviousTagSize.
func (j *flvJoiner) writeTag(tag [flvTagHeaderSize]byte, data []byte, ts int64) error {
	stamp := uint32(min(max(ts, 0), math.MaxUint32))
	tag[4], tag[5], tag[6], tag[7] = byte(stamp>>16), byte(stamp>>8), byte(stamp), byte(stamp>>24)
	if tag[0] == flvTagScript && j.durationAt < 0 && bytes.HasPrefix(data, []byte("\x02\x00\x0aonMetaData")) {
		// Property names in the AMF0 object are u16-length strings, and a
		// number value is marker 0 then a big-endian float64.
		dataAt := j.written + flvTagHeaderSize
		if i := bytes.Index(data, []byte("\x00\x08duration\x00")); i >= 0 && i+19 <= len(data) {
			j.durationAt = dataAt + int64(i) + 11
		}
		if i := bytes.Index(data, []byte("\x00\x08filesize\x00")); i >= 0 && i+19 <= len(data) {
			j.filesizeAt = dataAt + int64(i) + 11
		}
	}
	var trailer [4]byte
	binary.BigEndian.PutUint32(trailer[:], uint32(flvTagHeaderSize+len(data)))
	if err := j.write(tag[:], data, trailer[:]); err != nil {
		return err
	}
	if j.tags == 0 {
		j.firstTS = stamp
	}
	j.lastTS = max(j.lastTS, stamp)
	j.media = j.media || tag[0] != flvTagScript
	j.tags++
	return nil
}

func (j *flvJoiner) write(parts ...[]byte) error {
	for _, part := range parts {
		n, err := j.w.Write(part)
		j.written += int64(n)
		if err != nil {
			return err
		}
	}
	return nil
}

func (j *flvJoiner) buffer(size int) []byte {
	if cap(j.buf) < size {
		j.buf = make([]byte, size)
	}
	return j.buf[:size]
}

// flvTimestamp is a tag's timestamp: 24 bits, then the extension byte as
// the upper 8.
func flvTimestamp(tag [flvTagHeaderSize]byte) uint32 {
	return uint32(tag[7])<<24 | uint32(tag[4])<<16 | uint32(tag[5])<<8 | uint32(tag[6])
}

// endOfData reports whether a read ended at the end of the input, whole or
// cut short, rather than failing.
func endOfData(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}
