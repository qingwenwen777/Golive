package service

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testFLVTag is one tag of a synthetic FLV file.
type testFLVTag struct {
	kind byte
	ts   uint32
	data []byte
}

// buildTestFLV returns an FLV file holding tags, laid out as SRS's DVR
// writes it.
func buildTestFLV(tags ...testFLVTag) []byte {
	var b bytes.Buffer
	b.Write([]byte{'F', 'L', 'V', 1, 0x05, 0, 0, 0, flvHeaderSize, 0, 0, 0, 0})
	for _, tag := range tags {
		size := len(tag.data)
		b.Write([]byte{tag.kind, byte(size >> 16), byte(size >> 8), byte(size),
			byte(tag.ts >> 16), byte(tag.ts >> 8), byte(tag.ts), byte(tag.ts >> 24), 0, 0, 0})
		b.Write(tag.data)
		_ = binary.Write(&b, binary.BigEndian, uint32(flvTagHeaderSize+size))
	}
	return b.Bytes()
}

// readTestFLV parses an FLV file, failing the test unless it is one header
// followed by whole tags, each with its correct PreviousTagSize.
func readTestFLV(t *testing.T, data []byte) []testFLVTag {
	t.Helper()
	require.GreaterOrEqual(t, len(data), flvHeaderSize+4)
	require.Equal(t, "FLV", string(data[:3]))
	require.Equal(t, uint32(flvHeaderSize), binary.BigEndian.Uint32(data[5:9]))
	require.Zero(t, binary.BigEndian.Uint32(data[9:13]), "PreviousTagSize0")
	var tags []testFLVTag
	for pos := flvHeaderSize + 4; pos < len(data); {
		require.LessOrEqual(t, pos+flvTagHeaderSize, len(data), "tag header cut short")
		var header [flvTagHeaderSize]byte
		copy(header[:], data[pos:])
		require.Contains(t, []byte{flvTagAudio, flvTagVideo, flvTagScript}, header[0], "tag type at %d", pos)
		size := int(header[1])<<16 | int(header[2])<<8 | int(header[3])
		end := pos + flvTagHeaderSize + size
		require.LessOrEqual(t, end+4, len(data), "tag at %d cut short", pos)
		require.Equal(t, uint32(flvTagHeaderSize+size), binary.BigEndian.Uint32(data[end:end+4]), "PreviousTagSize of tag at %d", pos)
		tags = append(tags, testFLVTag{kind: header[0], ts: flvTimestamp(header), data: data[pos+flvTagHeaderSize : end]})
		pos = end + 4
	}
	return tags
}

// testMetadata is an onMetaData script tag the way SRS writes it: an AMF0
// object ending in filesize and duration.
func testMetadata(width float64) testFLVTag {
	var b bytes.Buffer
	b.Write([]byte("\x02\x00\x0aonMetaData\x03"))
	for _, prop := range []struct {
		name  string
		value float64
	}{{"width", width}, {"filesize", 0}, {"duration", 0}} {
		_ = binary.Write(&b, binary.BigEndian, uint16(len(prop.name)))
		b.WriteString(prop.name)
		b.WriteByte(0)
		_ = binary.Write(&b, binary.BigEndian, math.Float64bits(prop.value))
	}
	b.Write([]byte{0, 0, 9})
	return testFLVTag{kind: flvTagScript, data: b.Bytes()}
}

// metadataNumber reads the number property name of an onMetaData tag.
func metadataNumber(t *testing.T, tag testFLVTag, name string) float64 {
	t.Helper()
	key := append([]byte{0, byte(len(name))}, name...)
	i := bytes.Index(tag.data, append(key, 0))
	require.GreaterOrEqual(t, i, 0, name)
	at := i + len(key) + 1
	return math.Float64frombits(binary.BigEndian.Uint64(tag.data[at : at+8]))
}

// testSession is a publish session's tags: metadata, the AVC and AAC
// sequence headers (seq tells sessions apart, as the encoder settings may
// change), then frames every 20ms, alternating video and audio, until last.
func testSession(seq byte, last uint32) []testFLVTag {
	tags := []testFLVTag{
		testMetadata(float64(seq)),
		{kind: flvTagVideo, data: []byte{0x17, 0x00, 0, 0, 0, 0x01, seq}},
		{kind: flvTagAudio, data: []byte{0xaf, 0x00, 0x12, seq}},
	}
	for ts := uint32(0); ts <= last; ts += 20 {
		if ts%40 == 0 {
			tags = append(tags, testFLVTag{kind: flvTagVideo, ts: ts, data: []byte{0x27, 0x01, 0, 0, 0, seq, byte(ts)}})
		} else {
			tags = append(tags, testFLVTag{kind: flvTagAudio, ts: ts, data: []byte{0xaf, 0x01, seq, byte(ts)}})
		}
	}
	return tags
}

func writeTestFiles(t *testing.T, files ...[]byte) []string {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for i, data := range files {
		path := filepath.Join(dir, "live-a."+string(rune('1'+i))+"000.flv")
		require.NoError(t, os.WriteFile(path, data, 0o644))
		paths = append(paths, path)
	}
	return paths
}

func joinTestFiles(t *testing.T, inputs []string) (flvJoin, []byte, error) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "joined-*.flv")
	require.NoError(t, err)
	joined, joinErr := joinFLV(out, inputs)
	require.NoError(t, out.Close())
	data, err := os.ReadFile(out.Name())
	require.NoError(t, err)
	return joined, data, joinErr
}

func TestJoinFLVJoinsSessions(t *testing.T) {
	first, second := testSession(1, 1000), testSession(2, 500)
	inputs := writeTestFiles(t, buildTestFLV(first...), buildTestFLV(second...))

	joined, data, err := joinTestFiles(t, inputs)

	require.NoError(t, err)
	require.Empty(t, joined.damaged)
	tags := readTestFLV(t, data) // one header, then whole tags with correct sizes
	// The second session's metadata is dropped; its sequence headers stay.
	want := append(append([]testFLVTag{}, first...), second[1:]...)
	require.Len(t, tags, len(want))
	require.Equal(t, len(tags), joined.tags)
	require.Equal(t, int64(len(data)), joined.size)
	for i := range want {
		require.Equal(t, want[i].kind, tags[i].kind, "tag %d", i)
		if i > 0 { // the metadata is checked below
			require.Equal(t, want[i].data, tags[i].data, "tag %d", i)
		}
	}
	// The second session continues flvSegmentGap ms after the first.
	for i, tag := range tags {
		wantTS := want[i].ts
		if i >= len(first) {
			wantTS += 1000 + flvSegmentGap
		}
		require.Equal(t, wantTS, tag.ts, "tag %d", i)
		if i > 0 {
			require.GreaterOrEqual(t, tag.ts, tags[i-1].ts, "tag %d goes back in time", i)
		}
	}
	require.Equal(t, 1540*time.Millisecond, joined.duration)
	require.Equal(t, 1.54, metadataNumber(t, tags[0], "duration"))
	require.Equal(t, float64(len(data)), metadataNumber(t, tags[0], "filesize"))
	require.Equal(t, 1.0, metadataNumber(t, tags[0], "width"))
}

func TestJoinFLVUsesExtendedTimestamps(t *testing.T) {
	// A session longer than the 24 bits of a timestamp (4h39m) sets the
	// extension byte.
	long := uint32(1<<24 + 500)
	first := []testFLVTag{
		{kind: flvTagVideo, ts: 0, data: []byte{0x17, 0x01, 1}},
		{kind: flvTagVideo, ts: long, data: []byte{0x27, 0x01, 2}},
	}
	second := []testFLVTag{{kind: flvTagVideo, ts: 0, data: []byte{0x17, 0x01, 3}}}
	inputs := writeTestFiles(t, buildTestFLV(first...), buildTestFLV(second...))

	_, data, err := joinTestFiles(t, inputs)

	require.NoError(t, err)
	tags := readTestFLV(t, data)
	require.Len(t, tags, 3)
	require.Equal(t, long, tags[1].ts)
	require.Equal(t, long+flvSegmentGap, tags[2].ts)
	// 24 bits, then the upper 8 in the extension byte.
	last := data[len(data)-4-len(tags[2].data)-flvTagHeaderSize:]
	require.Equal(t, []byte{0x00, 0x02, 0x1c, 0x01}, last[4:8])
}

func TestJoinFLVKeepsTagsBeforeATruncatedTag(t *testing.T) {
	first, second := testSession(1, 200), testSession(2, 100)
	crashed := buildTestFLV(first...)
	// SRS died in the middle of writing the last tag's data.
	crashed = crashed[:len(crashed)-4-2]
	headerCut := buildTestFLV(first...)
	headerCut = headerCut[:len(headerCut)-4-len(first[len(first)-1].data)-5]
	trailerCut := buildTestFLV(first...)
	trailerCut = trailerCut[:len(trailerCut)-2]
	for name, cut := range map[string]struct {
		file []byte
		kept int // tags of the first file that survive
	}{
		"in tag data":             {crashed, len(first) - 1},
		"in tag header":           {headerCut, len(first) - 1},
		"in last PreviousTagSize": {trailerCut, len(first)},
	} {
		t.Run(name, func(t *testing.T) {
			inputs := writeTestFiles(t, cut.file, buildTestFLV(second...))

			joined, data, err := joinTestFiles(t, inputs)

			require.NoError(t, err)
			require.Len(t, joined.damaged, 1)
			require.Contains(t, joined.damaged[0], inputs[0])
			tags := readTestFLV(t, data)
			require.Len(t, tags, cut.kept+len(second)-1)
			require.Equal(t, 1.0, metadataNumber(t, tags[0], "width"))
			for i := 1; i < cut.kept; i++ {
				require.Equal(t, first[i].data, tags[i].data, "tag %d", i)
			}
			lastKept := first[cut.kept-1].ts
			require.Equal(t, lastKept+flvSegmentGap, tags[cut.kept].ts)
			require.Equal(t, second[1].data, tags[cut.kept].data)
		})
	}
}

func TestJoinFLVSkipsFilesThatAreNotFLV(t *testing.T) {
	session := testSession(1, 100)
	inputs := writeTestFiles(t, []byte("not a recording"), buildTestFLV(session...), buildTestFLV())

	joined, data, err := joinTestFiles(t, inputs)

	require.NoError(t, err)
	require.Len(t, joined.damaged, 1)
	require.Len(t, readTestFLV(t, data), len(session))

	_, _, err = joinTestFiles(t, writeTestFiles(t, []byte("not a recording"), buildTestFLV()))
	require.ErrorContains(t, err, "no audio or video")
}
