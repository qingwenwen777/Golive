// Package miclink holds the Redis contract shared by gift-service, which
// issues mic-link publish tokens to approved guests, and room-service, which
// checks them in the SRS on_publish hook. Both services use the same Redis.
package miclink

// StreamPrefix marks the SRS streams mic-link guests publish their audio to.
const StreamPrefix = "miclink-"

// StreamName is the SRS stream a guest publishes to. It must match
// micStreamName in golive-web/src/lib/micRtc.ts.
func StreamName(roomID, userID string) string {
	return StreamPrefix + roomID + "-" + userID
}

// TokenKey is the Redis key holding the publish token bound to stream.
func TokenKey(stream string) string {
	return "miclink:token:" + stream
}
