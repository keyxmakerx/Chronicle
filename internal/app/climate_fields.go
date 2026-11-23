package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// climate_fields.go gives the Location categories that existed before places
// had a climate their Climate field, once per site. Categories made later
// get it from the starting sets and genre presets they are made from; after
// the sweep an owner who deletes the field keeps it deleted.

// climateFieldsKey is the site setting that records the sweep ran.
const climateFieldsKey = "reconcile.climate_fields_added"

// climateFieldEnsurer is the slice of the entity service the sweep needs.
type climateFieldEnsurer interface {
	EnsureClimateFields(ctx context.Context) (int, error)
}

// addClimateFieldsOnce runs EnsureClimateFields unless it already ran on
// this site. The setting is written only when the sweep reached every
// category, so a crash part-way or a category that failed repeats it on the
// next start, which is harmless: a category that has the field is left alone.
func addClimateFieldsOnce(ctx context.Context, st pageExtrasSettings, ents climateFieldEnsurer) (int, error) {
	if v, err := st.Get(ctx, climateFieldsKey); err == nil && v == "1" {
		return 0, nil
	} else if err != nil {
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != 404 {
			return 0, fmt.Errorf("reading %s: %w", climateFieldsKey, err)
		}
	}
	n, err := ents.EnsureClimateFields(ctx)
	if err != nil {
		return n, err
	}
	if err := st.Set(ctx, climateFieldsKey, "1"); err != nil {
		return n, fmt.Errorf("recording %s: %w", climateFieldsKey, err)
	}
	return n, nil
}
