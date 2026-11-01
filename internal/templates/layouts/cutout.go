package layouts

import (
	"os"
	"sync"
)

// CutoutDir is where tools/fetch_cutout.py puts the token maker's smart
// cutout (the model and onnxruntime-web), under the static root. It is
// fetched by the Docker build, not kept in git, so a checkout without
// `make cutout` has none.
const CutoutDir = "/static/vendor/cutout/"

// CutoutModel is the cutout model's file name in CutoutDir.
const CutoutModel = "isnet-general-use-q8.onnx"

// CutoutReady reports whether the smart cutout is installed. The token maker
// offers "Cut out the figure" only when it is; the answer cannot change while
// the process runs, so it is looked up once.
var CutoutReady = sync.OnceValue(func() bool {
	for _, f := range []string{CutoutModel, "ort.wasm.min.js", "ort-wasm-simd-threaded.mjs", "ort-wasm-simd-threaded.wasm"} {
		if st, err := os.Stat("static/vendor/cutout/" + f); err != nil || st.Size() == 0 {
			return false
		}
	}
	return true
})
