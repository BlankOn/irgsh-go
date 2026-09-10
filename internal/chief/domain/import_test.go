package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An irgsh-cli of 2.1.0 or older names the source suite in "dist" and our
// distribution in "targetDist". Reading that payload as-is would route the
// job to a queue named after the source suite, where nothing consumes it.
func TestImportSubmission_NormalizeLegacyPayload(t *testing.T) {
	var submission ImportSubmission
	require.NoError(t, json.Unmarshal([]byte(
		`{"sourceUrl":"http://deb.debian.org/debian","dist":"sid","targetDist":"verbeek"}`), &submission))

	submission.Normalize()

	assert.Equal(t, "verbeek", submission.Dist, "dist must end up naming our distribution")
	assert.Equal(t, "sid", submission.SourceDist)
	assert.Empty(t, submission.TargetDist, "the legacy field must not be forwarded")
}

func TestImportSubmission_NormalizeLeavesCurrentPayloadAlone(t *testing.T) {
	var submission ImportSubmission
	require.NoError(t, json.Unmarshal([]byte(
		`{"sourceUrl":"http://deb.debian.org/debian","dist":"verbeek","sourceDist":"sid"}`), &submission))

	submission.Normalize()

	assert.Equal(t, "verbeek", submission.Dist)
	assert.Equal(t, "sid", submission.SourceDist)
}

// Normalizing twice must not swap the fields back.
func TestImportSubmission_NormalizeIsIdempotent(t *testing.T) {
	submission := ImportSubmission{Dist: "sid", TargetDist: "verbeek"}

	submission.Normalize()
	submission.Normalize()

	assert.Equal(t, "verbeek", submission.Dist)
	assert.Equal(t, "sid", submission.SourceDist)
}

// A payload carrying neither is left for the caller's own validation to reject.
func TestImportSubmission_NormalizeWithoutEitherField(t *testing.T) {
	submission := ImportSubmission{Dist: "verbeek"}

	submission.Normalize()

	assert.Equal(t, "verbeek", submission.Dist)
	assert.Empty(t, submission.SourceDist)
}
