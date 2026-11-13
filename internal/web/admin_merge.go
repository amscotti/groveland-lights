// Photo attach for the duplicate merge (A-205): dup_choice=merge must apply
// the new submission's label AND photo onto the surviving row. The write goes
// through store.EditLightPhoto so the store keeps ownership of the schema and
// its invariants (live-row guard, foreign key, updated_at stamp).
package web

import (
	"context"

	"github.com/amscotti/groveland-lights/internal/store"
)

// attachPhoto points one live light at the merged submission's photo row.
// A missing or soft-deleted light wraps store.ErrNotFound.
func (s *Server) attachPhoto(ctx context.Context, id store.LightID, photo store.PhotoID) error {
	_, err := s.Store.EditLightPhoto(ctx, id, photo)
	return err
}
