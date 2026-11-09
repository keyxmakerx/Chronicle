package crafting

import (
	"io"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// The routes for making recipes where the work is: an item's page, the
// Homebrew Workshop's Crafting recipes part, its generator and recipe packs.

// ItemCrafting handles GET /campaigns/:id/crafting/items/:eid.
func (h *Handler) ItemCrafting(c *echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.ItemCrafting(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// CreateMaterial handles POST /campaigns/:id/crafting/materials.
func (h *Handler) CreateMaterial(c *echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in MaterialInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.CreateMaterial(c.Request().Context(), cc.Campaign.ID, v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// ItemBlock handles GET /campaigns/:id/crafting/item-block.
func (h *Handler) ItemBlock(c *echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	placed, err := h.svc.ItemBlock(c.Request().Context(), cc.Campaign.ID, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]bool{"placed": placed})
}

// PlaceItemBlock handles POST /campaigns/:id/crafting/item-block.
func (h *Handler) PlaceItemBlock(c *echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	n, err := h.svc.PlaceItemBlock(c.Request().Context(), cc.Campaign.ID, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]int{"changed": n})
}

// Generate handles POST /campaigns/:id/crafting/generate.
func (h *Handler) Generate(c *echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in GenerateInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.Generate(c.Request().Context(), cc.Campaign.ID, v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// ListPacks handles GET /campaigns/:id/crafting/packs.
func (h *Handler) ListPacks(c *echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.ListPacks(c.Request().Context(), cc.Campaign.ID, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// SavePack handles POST /campaigns/:id/crafting/packs.
func (h *Handler) SavePack(c *echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in SavePackInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.SavePack(c.Request().Context(), cc.Campaign.ID, v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// UploadPack handles POST /campaigns/:id/crafting/packs/file: the pack
// file as the request body, up to maxPackBytes.
func (h *Handler) UploadPack(c *echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxPackBytes+1))
	if err != nil || len(raw) == 0 {
		return apperror.NewBadRequest("the file is missing")
	}
	out, err := h.svc.UploadPack(c.Request().Context(), cc.Campaign.ID, v, raw)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// PackDetail handles GET /campaigns/:id/crafting/packs/:pid.
func (h *Handler) PackDetail(c *echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.PackDetail(c.Request().Context(), cc.Campaign.ID, c.Param("pid"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// UpdatePack handles PUT /campaigns/:id/crafting/packs/:pid.
func (h *Handler) UpdatePack(c *echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in PackChange
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.UpdatePack(c.Request().Context(), cc.Campaign.ID, c.Param("pid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// DeletePack handles DELETE /campaigns/:id/crafting/packs/:pid.
func (h *Handler) DeletePack(c *echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if err := h.svc.DeletePack(c.Request().Context(), cc.Campaign.ID, c.Param("pid"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// PackFile handles GET /campaigns/:id/crafting/packs/:pid/file: the pack as
// a download.
func (h *Handler) PackFile(c *echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	raw, name, err := h.svc.PackFile(c.Request().Context(), cc.Campaign.ID, c.Param("pid"), v)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	return c.Blob(http.StatusOK, "application/json", raw)
}

// ImportPack handles POST /campaigns/:id/crafting/packs/:pid/import.
func (h *Handler) ImportPack(c *echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in ImportInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.ImportPack(c.Request().Context(), cc.Campaign.ID, c.Param("pid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}
