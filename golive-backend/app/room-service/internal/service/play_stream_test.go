package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecordingStreamNameFollowsPlayName(t *testing.T) {
	ctx := context.Background()
	env := newLifecycleTestEnv(t)
	replay, _ := env.withReplay(t)
	st := startTestLive(t, env.svc, "owner-recording-name")
	room, err := env.rooms.GetByID(ctx, st.ID)
	require.NoError(t, err)
	record := func(name string) {
		require.NoError(t, os.WriteFile(filepath.Join(replay.recordDir, name), []byte("flv"), 0o644))
	}

	// SRS names the DVR file after the stream OBS publishes to.
	play := publishStream(st.StreamKey)
	require.NotEqual(t, room.ID, play)
	record(play + ".flv")
	require.Equal(t, play, replay.recordingStreamName(*room))

	// Rooms that went live under an earlier stream name keep their recording.
	record(room.ID + ".flv")
	require.Equal(t, room.ID, replay.recordingStreamName(*room))
	record(room.StreamKey + ".flv")
	require.Equal(t, room.StreamKey, replay.recordingStreamName(*room))

	room.StreamKey = ""
	require.Equal(t, room.ID, replay.recordingStreamName(*room))
}
