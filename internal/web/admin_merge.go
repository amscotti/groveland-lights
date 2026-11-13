// Duplicate merge (A-205): dup_choice=merge applies the new submission's label
// AND photo onto the surviving row, never creating a second row.
package web

import (
	"errors"
	"net/http"

	"github.com/amscotti/groveland-lights/internal/geo"
	"github.com/amscotti/groveland-lights/internal/store"
)

// mergeDuplicate applies the new submission's label and photo onto the
// existing row. With a photo the write goes through store.EditLightPhoto,
// which updates the label, asset number and photo in one transaction: if the
// photo write fails, the label is rolled back too. Without a photo the plain
// EditLight path is used.
//
// The replaced photo row/file is intentionally not deleted here: A-207's
// no-stray-file contract covers a FAILED upload (the pipeline removes its own
// file), while garbage-collecting a replaced, still-served photo is a separate
// retention decision.
func (s *Server) mergeDuplicate(w http.ResponseWriter, r *http.Request, sub walkSubmission, decision geo.DupDecision, photo *UploadedPhoto) {
	existing, err := s.Store.GetLight(r.Context(), store.LightID(decision.ExistingID))
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			s.Log.Error("admin merge: read existing", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		// The duplicate vanished between the check and the merge: save as a
		// plain new light instead (keeping the uploaded photo).
		_, cerr := s.Store.CreateLight(r.Context(), store.NewLight{
			AssetNo:   sub.AssetNo,
			Kind:      sub.Kind,
			Label:     sub.Label,
			Lat:       sub.Lat,
			Lon:       sub.Lon,
			AccuracyM: sub.AccuracyM,
			PhotoID:   photoIDOf(photo),
		})
		if cerr != nil {
			s.Log.Error("admin merge: create", "err", cerr)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		s.respondWalkOK(w, r)
		return
	}
	if photo != nil {
		_, err = s.Store.EditLightPhoto(r.Context(), existing.ID, sub.Label, existing.AssetNo, photo.ID)
	} else {
		_, err = s.Store.EditLight(r.Context(), existing.ID, sub.Label, existing.AssetNo)
	}
	if err != nil {
		s.Log.Error("admin merge", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.respondWalkOK(w, r)
}
