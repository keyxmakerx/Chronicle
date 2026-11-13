# pack.py [stills|shelf|clips] [items] : paints the neutral renders and their
# colour-role masks and writes the recolourable pictures Chronicle serves.
# Every picture is an atlas of four tiles: the item painted in neutral greys
# (top left), then how much of each pixel takes the body, trim and detail
# colour (top right, bottom left, bottom right), each as a grey picture so
# the image codec keeps them sharp.
#   s/<k>.webp      256 tiles, the picture in lists and tiles
#   l/<k>.webp      512 tiles, an opened item
#   shelf/<k>.webp  128 tiles on a clear background, for shop shelves
#   move/<k>.webp   64 frames of 256, eight across, each with its masks at
#                   half size beside it
import sys, os, glob, numpy as np
from multiprocessing import Pool
from PIL import Image, ImageFilter
from paintm import paint
D = os.path.dirname(os.path.abspath(__file__)) + '/'
OUT = D + 'pub2/'
def atlas(rgb, mask, size, alpha=None):
    t = lambda a, mode: Image.fromarray(a.astype(np.uint8), mode).resize((size, size), Image.LANCZOS)
    a = Image.new('RGBA' if alpha is not None else 'RGB', (size * 2, size * 2), (0, 0, 0, 255) if alpha is not None else (0, 0, 0))
    n = rgb.resize((size, size), Image.LANCZOS)
    if alpha is not None: n = n.convert('RGBA'); n.putalpha(t(alpha, 'L'))
    a.paste(n, (0, 0))
    for i, (x, y) in enumerate([(size, 0), (0, size), (size, size)]):
        g = t(np.clip(mask[..., i], 0, 255), 'L').convert(a.mode)
        a.paste(g, (x, y))
    return a
def load(p): return np.asarray(Image.open(p).convert('RGBA'))
def still(k):
    n = load(D + 'out/still/%s-n.png' % k)[..., :3]; m = load(D + 'out/still/%s-m.png' % k)[..., :3]
    rgb, mk = paint(n, m, 5)
    atlas(rgb, mk, 512).save(OUT + 'l/%s.webp' % k, quality=82, method=6)
    atlas(rgb, mk, 256).save(OUT + 's/%s.webp' % k, quality=84, method=6)
    return k
def shelf(k):
    n = load(D + 'out/shelf/%s-n.png' % k).astype(np.float64); m = load(D + 'out/shelf/%s-m.png' % k)[..., :3]
    al = n[..., 3:] / 255.0; over = n[..., :3] * al + 12 * (1 - al)      # painted over the dark it will sit on
    rgb, mk = paint(over, m, 4)
    alpha = np.asarray(Image.fromarray(n[..., 3].astype(np.uint8)).filter(ImageFilter.GaussianBlur(.6)))
    # The ground shadow can reach the edge of the picture; fade it out there so a
    # shelf never shows the square the picture was cut from.
    h, w = alpha.shape; e = .12
    ry = np.clip(np.minimum(np.arange(h), h - 1 - np.arange(h)) / (h * e), 0, 1)
    rx = np.clip(np.minimum(np.arange(w), w - 1 - np.arange(w)) / (w * e), 0, 1)
    ramp = np.outer(ry, rx); ramp = ramp * ramp * (3 - 2 * ramp)
    alpha = (alpha * ramp).astype(np.uint8)
    atlas(rgb, mk, 128, alpha).save(OUT + 'shelf/%s.webp' % k, quality=86, method=6, alpha_quality=90)
    return k
def frame(rgb, mask, size):
    # One frame of a moving item: the neutral picture, then its three masks
    # at half size in a square beside it (the fourth corner unused).
    h = size // 2; a = Image.new('RGB', (size * 2, size))
    a.paste(rgb.resize((size, size), Image.LANCZOS), (0, 0))
    for i, (x, y) in enumerate([(size, 0), (size + h, 0), (size, h)]):
        a.paste(Image.fromarray(np.clip(mask[..., i], 0, 255).astype(np.uint8), 'L').resize((h, h), Image.LANCZOS).convert('RGB'), (x, y))
    return a
def mask_at(p, shape):
    # Clip masks are drawn at half size (they are served at a quarter) and
    # brought up to the picture's size so the paint pass can share strokes.
    m = Image.open(p).convert('RGB')
    if m.size != (shape[1], shape[0]): m = m.resize((shape[1], shape[0]), Image.BILINEAR)
    return np.asarray(m).astype(np.float64)
def clip(k):
    fs = sorted(glob.glob(D + 'out/clip/%s-??-n.png' % k))
    raw = [load(f)[..., :3].astype(np.float64) for f in fs]; msk = [mask_at(f.replace('-n.png', '-m.png'), raw[0].shape) for f in fs]
    still = np.median(np.stack(raw), axis=0); mstill = np.median(np.stack(msk), axis=0)
    b_rgb, b_m = paint(still.astype(np.uint8), mstill, 3); b_rgb = np.asarray(b_rgb).astype(np.float64)
    cols, size = 8, 256; sheet = Image.new('RGB', (cols * size * 2, (len(raw) + cols - 1) // cols * size))
    for i, (a, mm) in enumerate(zip(raw, msk)):
        # Only what changed between frames is repainted, so the still parts do not shimmer.
        diff = np.maximum(np.abs(a - still).max(axis=2), np.abs(mm - mstill).max(axis=2))
        w = np.asarray(Image.fromarray(np.clip(diff * 8, 0, 255).astype(np.uint8)).filter(ImageFilter.GaussianBlur(5))).astype(np.float64) / 255.0
        w = np.clip(w * 3.0, 0, 1)[..., None]
        p_rgb, p_m = paint(a.astype(np.uint8), mm, 3); p_rgb = np.asarray(p_rgb).astype(np.float64)
        rgb = Image.fromarray(np.clip(b_rgb + (p_rgb - b_rgb) * w, 0, 255).astype(np.uint8)); mk = b_m + (p_m - b_m) * w
        sheet.paste(frame(rgb, mk, size), ((i % cols) * size * 2, (i // cols) * size))
    sheet.save(OUT + 'move/%s.webp' % k, quality=74, method=5)
    return k
if __name__ == '__main__':
    mode, only = sys.argv[1], sys.argv[2:]
    for d in ('s', 'l', 'shelf', 'move'): os.makedirs(OUT + d, exist_ok=True)
    if mode == 'clips': ks = only or sorted({os.path.basename(f)[:-9] for f in glob.glob(D + 'out/clip/*-63-n.png')})
    else: ks = only or [os.path.basename(f)[:-6] for f in sorted(glob.glob(D + 'out/%s/*-m.png' % ('still' if mode == 'stills' else 'shelf')))]
    fn = {'stills': still, 'shelf': shelf, 'clips': clip}[mode]
    with Pool(4) as p:
        for k in p.imap_unordered(fn, ks): print(k, flush=True)
