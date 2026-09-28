package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

const unnamedID = "3f2a0c1e-9b7d-4c1a-8e2f-0a1b2c3d4e5f"

// Names used to fall back to an English "Creator <id prefix>" (or the id
// itself); now they are "" and the apps label them in the viewer's language.
func TestNamesNeverFallBackToAnID(t *testing.T) {
	require.Equal(t, "Luna", cleanDisplayName("  Luna "))
	require.Empty(t, cleanDisplayName(unnamedID))
	require.Empty(t, cleanDisplayName("  "))

	require.Equal(t, "Luna", recommendedCreatorName(repo.CreatorRecommendationCandidate{ID: unnamedID, Channel: unnamedID, DisplayName: "Luna"}))
	require.Empty(t, recommendedCreatorName(repo.CreatorRecommendationCandidate{ID: unnamedID, Channel: unnamedID, Username: unnamedID}))

	require.Equal(t, "luna", ownerProfileName(repo.OwnerProfile{ID: unnamedID, Username: "luna"}))
	require.Empty(t, ownerProfileName(repo.OwnerProfile{ID: unnamedID, Username: unnamedID}))
	require.Empty(t, ownerProfileName(repo.OwnerProfile{}))

	require.Equal(t, PostAuthor{ID: unnamedID}, fallbackPostAuthor(unnamedID))
	require.Equal(t, "luna", nonEmpty(" ", "luna"))
	require.Empty(t, nonEmpty("", " "))
}
