package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/UsmanXTech/shorty/internal/links"
	"github.com/UsmanXTech/shorty/internal/variants"
)

type VariantAPI struct {
	links     links.Repository
	variants  variants.Store
	listCache *variants.ListCache
}

// WithListCache shares the redirect handler's variant-list cache so the
// hot path avoids a database query per click. Mutations invalidate it.
func (a *VariantAPI) WithListCache(c *variants.ListCache) *VariantAPI {
	a.listCache = c
	return a
}

func NewVariantAPI(linkRepo links.Repository, store variants.Store) *VariantAPI {
	return &VariantAPI{links: linkRepo, variants: store}
}

func (a *VariantAPI) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/links/{id}/variants", a.list)
	mux.HandleFunc("POST /api/v1/links/{id}/variants", a.create)
	mux.HandleFunc("PUT /api/v1/links/{id}/variants/{variant_id}", a.update)
	mux.HandleFunc("DELETE /api/v1/links/{id}/variants/{variant_id}", a.delete)
}

func (a *VariantAPI) scopedLink(r *http.Request) (links.Link, bool) {
	id, ok := pathID(r)
	if !ok {
		return links.Link{}, false
	}
	link, err := a.links.GetByID(id)
	if err != nil {
		return links.Link{}, false
	}
	if team, ok := teamFromRequest(r); ok && link.TeamID != team.ID {
		return links.Link{}, false
	}
	return link, true
}

type variantRequest struct {
	URL string `json:"url"`
	// Weight uses omitted-vs-explicit semantics: omitted preserves (update)
	// or defaults to 1 (create); 0 disables the variant; null resets to 1.
	Weight optInt `json:"weight,omitempty"`
}

func (a *VariantAPI) list(w http.ResponseWriter, r *http.Request) {
	link, ok := a.scopedLink(r)
	if !ok {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	list, err := a.variants.List(link.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list variants")
		return
	}
	if list == nil {
		list = []variants.Variant{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *VariantAPI) create(w http.ResponseWriter, r *http.Request) {
	link, ok := a.scopedLink(r)
	if !ok {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	var req variantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if !validURL(req.URL) {
		writeError(w, http.StatusBadRequest, "url must be an absolute http or https URL")
		return
	}
	weight := 1
	if req.Weight.Set {
		if req.Weight.Value == nil {
			weight = 1
		} else {
			weight = int(*req.Weight.Value)
		}
	}
	if weight < 0 {
		writeError(w, http.StatusBadRequest, "weight must not be negative")
		return
	}
	v, err := a.variants.Create(link.ID, req.URL, weight)
	if errors.Is(err, variants.ErrNoLink) {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create variant")
		return
	}
	a.listCache.Invalidate(link.ID)
	writeJSON(w, http.StatusCreated, v)
}

func (a *VariantAPI) update(w http.ResponseWriter, r *http.Request) {
	link, ok := a.scopedLink(r)
	if !ok {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	vid, ok := pathVariantID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid variant id")
		return
	}
	existing, err := a.variants.Get(vid)
	if err != nil {
		if errors.Is(err, variants.ErrNotFound) {
			writeError(w, http.StatusNotFound, "variant not found")
		} else {
			writeError(w, http.StatusInternalServerError, "could not get variant")
		}
		return
	}
	if existing.LinkID != link.ID {
		writeError(w, http.StatusNotFound, "variant not found")
		return
	}
	var req variantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.URL != "" && !validURL(req.URL) {
		writeError(w, http.StatusBadRequest, "url must be an absolute http or https URL")
		return
	}
	// -1 preserves the current weight; 0 disables the variant.
	weight := -1
	if req.Weight.Set {
		if req.Weight.Value == nil {
			weight = 1
		} else {
			weight = int(*req.Weight.Value)
		}
	}
	if weight < -1 {
		writeError(w, http.StatusBadRequest, "weight must not be negative")
		return
	}
	updated, err := a.variants.Update(vid, req.URL, weight)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update variant")
		return
	}
	a.listCache.Invalidate(link.ID)
	writeJSON(w, http.StatusOK, updated)
}

func (a *VariantAPI) delete(w http.ResponseWriter, r *http.Request) {
	link, ok := a.scopedLink(r)
	if !ok {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	vid, ok := pathVariantID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid variant id")
		return
	}
	existing, err := a.variants.Get(vid)
	if err != nil {
		if errors.Is(err, variants.ErrNotFound) {
			writeError(w, http.StatusNotFound, "variant not found")
		} else {
			writeError(w, http.StatusInternalServerError, "could not get variant")
		}
		return
	}
	if existing.LinkID != link.ID {
		writeError(w, http.StatusNotFound, "variant not found")
		return
	}
	if err := a.variants.Delete(vid); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete variant")
		return
	}
	a.listCache.Invalidate(link.ID)
	w.WriteHeader(http.StatusNoContent)
}

func pathVariantID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("variant_id"), 10, 64)
	return id, err == nil && id > 0
}
