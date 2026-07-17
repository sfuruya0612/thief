package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/sfuruya0612/thief/backend/internal/snippet"
)

// handleSnippetsList は service の保存済みスニペットを更新日時の降順で返す。
func (s *Server) handleSnippetsList(w http.ResponseWriter, r *http.Request) {
	items, err := s.snippets.List(r.PathValue("service"))
	if err != nil {
		writeSnippetError(w, err)
		return
	}
	writeJSON(w, items)
}

// handleSnippetSave は service 配下にスニペットを作成または同名で上書きする。
func (s *Server) handleSnippetSave(w http.ResponseWriter, r *http.Request) {
	var req SnippetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid JSON body: "+err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || strings.TrimSpace(req.SQL) == "" {
		writeBadRequest(w, "name and sql are required")
		return
	}
	saved, err := s.snippets.Save(r.PathValue("service"), name, req.SQL)
	if err != nil {
		writeSnippetError(w, err)
		return
	}
	writeJSON(w, saved)
}

// handleSnippetDelete は service 配下の指定名のスニペットを削除する。
func (s *Server) handleSnippetDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.snippets.Delete(r.PathValue("service"), r.PathValue("name")); err != nil {
		writeSnippetError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeSnippetError はスニペット操作のエラーを HTTP ステータスへマップする。
func writeSnippetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, snippet.ErrInvalidService), errors.Is(err, snippet.ErrInvalidName):
		writeBadRequest(w, err.Error())
	case errors.Is(err, snippet.ErrNotFound):
		writeError(w, http.StatusNotFound, "SNIPPET_NOT_FOUND", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "SNIPPET_ERROR", err.Error())
	}
}
