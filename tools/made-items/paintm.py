# The paint pass carried onto a colour-role mask: each stroke is chosen from the
# picture's own colours, and the mask channels are averaged with the very same
# weights, so a recoloured picture keeps its brush strokes on the right parts.
import numpy as np
from PIL import Image, ImageFilter, ImageEnhance
def strokes(a, extra, r, q=8.0, N=8):
    h, w, _ = a.shape; pad = r + 1
    allc = np.concatenate([a, extra], axis=2)
    ap = np.pad(allc, ((pad, pad), (pad, pad), (0, 0)), mode='edge'); H, W = ap.shape[:2]; C = ap.shape[2]
    lum = ap[..., :3] @ np.array([.299, .587, .114])
    yy, xx = np.mgrid[-r:r + 1, -r:r + 1]; rad = np.hypot(xx, yy); ang = np.arctan2(yy, xx)
    gauss = np.exp(-(rad ** 2) / (2 * (r * .5) ** 2)) * (rad <= r + .5)
    F = np.fft.rfft2; Fa = [F(ap[..., c]) for c in range(C)]; Fl, Fl2 = F(lum), F(lum * lum)
    num = np.zeros_like(ap); den = np.zeros((H, W))
    for k in range(N):
        d = np.angle(np.exp(1j * (ang - 2 * np.pi * k / N))); ker = gauss * np.exp(-(d ** 2) / (2 * (np.pi / N * .85) ** 2)); ker[r, r] = gauss[r, r] / N; ker /= ker.sum()
        K = np.zeros((H, W)); K[:2 * r + 1, :2 * r + 1] = ker; FK = F(np.roll(K, (-r, -r), (0, 1)))
        inv = lambda X: np.fft.irfft2(X * FK, s=(H, W))
        ml = inv(Fl); sd = np.sqrt(np.maximum(inv(Fl2) - ml * ml, 0)); al = (sd + 1.5) ** (-q)
        for c in range(C): num[..., c] += inv(Fa[c]) * al
        den += al
    out = (num / den[..., None])[pad:pad + h, pad:pad + w]
    return out[..., :3], out[..., 3:]
def paint(rgb, mask, r):
    a = rgb.astype(np.float64); m = mask.astype(np.float64)
    b, mb = strokes(a, m, r); b, mb = strokes(b, mb, max(2, r // 2))
    b = np.clip(b * .78 + a * .22, 0, 255); mb = np.clip(mb * .78 + m * .22, 0, 255)
    g = b / 255.0; g = g ** .97; g[..., 0] *= 1.04; g[..., 2] *= .93
    g = np.clip((g - .5) * 1.06 + .5, 0, 1)
    out = Image.fromarray((g * 255).astype(np.uint8)).filter(ImageFilter.UnsharpMask(radius=2, percent=60, threshold=2))
    return ImageEnhance.Color(out).enhance(1.08), mb
