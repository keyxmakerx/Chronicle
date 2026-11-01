#!/usr/bin/env python3
"""Fetches the token maker's smart cutout into static/vendor/cutout/.

The cutout is IS-Net (Apache-2.0), run in the browser by onnxruntime-web
(MIT). Neither is kept in git: the model is 178 MB as published, so this
downloads the pinned original, checks it, and shrinks it to an int8 build of
about 46 MB, which must come out byte for byte as pinned. The Docker image
build runs it in its own stage; `make cutout` runs it for a local checkout.

Needs onnxruntime and onnx at the versions in REQUIREMENTS (the quantised
file's checksum depends on them).

    python3 tools/fetch_cutout.py [out-dir]
"""
import hashlib
import io
import os
import sys
import tarfile
import tempfile
import urllib.request

REQUIREMENTS = ["onnxruntime==1.31.1", "onnx==1.23.2"]

MODEL_URL = "https://github.com/danielgatis/rembg/releases/download/v0.0.0/isnet-general-use.onnx"
MODEL_SHA256 = "60920e99c45464f2ba57bee2ad08c919a52bbf852739e96947fbb4358c0d964a"
MODEL_OUT = "isnet-general-use-q8.onnx"
MODEL_OUT_SHA256 = "1f513a69287e05c689834d7a04d10de21bd9dca43565fd5a0e962383a2d58b21"

ORT_URL = "https://registry.npmjs.org/onnxruntime-web/-/onnxruntime-web-1.30.0.tgz"
ORT_SHA256 = "d2228df7e4616bc3348bf504ee888f3bec43789a273f0a63f3e68d203ce3bf71"
# The single-threaded WASM build and its loader; the page is not
# cross-origin isolated, so the threaded and WebGPU builds would go unused.
ORT_FILES = ["ort.wasm.min.js", "ort-wasm-simd-threaded.mjs", "ort-wasm-simd-threaded.wasm"]


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def fetch(url, want):
    with urllib.request.urlopen(url, timeout=300) as r:
        data = r.read()
    got = sha256(data)
    if got != want:
        sys.exit(f"fetch_cutout: {url} has sha256 {got}, expected {want}")
    return data


def main():
    out = sys.argv[1] if len(sys.argv) > 1 else os.path.join("static", "vendor", "cutout")
    os.makedirs(out, exist_ok=True)

    tgz = fetch(ORT_URL, ORT_SHA256)
    with tarfile.open(fileobj=io.BytesIO(tgz), mode="r:gz") as t:
        for name in ORT_FILES:
            f = t.extractfile("package/dist/" + name)
            if f is None:
                sys.exit(f"fetch_cutout: {name} missing from the onnxruntime-web package")
            with open(os.path.join(out, name), "wb") as w:
                w.write(f.read())

    from onnxruntime.quantization import QuantType, quantize_dynamic

    model = fetch(MODEL_URL, MODEL_SHA256)
    with tempfile.TemporaryDirectory() as tmp:
        src, dst = os.path.join(tmp, "fp32.onnx"), os.path.join(tmp, "q8.onnx")
        with open(src, "wb") as w:
            w.write(model)
        quantize_dynamic(src, dst, weight_type=QuantType.QUInt8)
        with open(dst, "rb") as r:
            q8 = r.read()
    got = sha256(q8)
    if got != MODEL_OUT_SHA256:
        sys.exit(f"fetch_cutout: the int8 model has sha256 {got}, expected {MODEL_OUT_SHA256}; "
                 f"check the versions in REQUIREMENTS ({', '.join(REQUIREMENTS)})")
    with open(os.path.join(out, MODEL_OUT), "wb") as w:
        w.write(q8)
    print(f"fetch_cutout: wrote {', '.join(ORT_FILES + [MODEL_OUT])} to {out}")


if __name__ == "__main__":
    main()
