package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"mctrl/internal/work"
)

func (s *Server) handleWorkArchive(w http.ResponseWriter, r *http.Request, id string) {
	var request struct {
		Confirm bool `json:"confirm"`
	}
	if err := readJSONBody(r, &request); err != nil || !request.Confirm {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Confirm archival with confirm: true", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	item, err := work.ArchiveMissing(ctx, s.works, s.tmux, id)
	switch {
	case errors.Is(err, work.ErrNotFound):
		writeError(w, http.StatusNotFound, "WORK_NOT_FOUND", "Work was not found", nil)
	case errors.Is(err, work.ErrArchiveUnsafe), errors.Is(err, work.ErrAttemptMismatch):
		writeError(w, http.StatusConflict, "WORK_ARCHIVE_UNSAFE", "The Session or processes still exist, their absence cannot be verified, or exit evidence needs reconciliation. Refresh and inspect the Work before archiving.", nil)
	case err != nil:
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not archive Work", nil)
	default:
		writeJSON(w, http.StatusOK, item.DTO())
	}
}
