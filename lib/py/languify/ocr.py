"""Batch OCR: turn a pile of page images into one text file each.

Uses pytesseract, a thin Python binding over the tesseract C++ engine, so
a whole batch runs as in-process calls into one already-loaded library
rather than spawning (and re-initializing) a fresh `tesseract` process per
image.
"""

import os
from typing import Callable, Iterable, List, Optional

from languify.config import LanguageConfig


def ocr_image(path: str, lang: LanguageConfig) -> str:
    try:
        import pytesseract
        from PIL import Image
    except ImportError as exc:
        raise ImportError(
            "pytesseract and pillow are required for OCR -- see "
            "languify_setup.txt"
        ) from exc

    with Image.open(path) as image:
        return pytesseract.image_to_string(image, lang=lang.ocr.lang_code)


def output_path_for(image_path: str, out_dir: Optional[str]) -> str:
    base = os.path.splitext(os.path.basename(image_path))[0] + ".txt"
    directory = out_dir or os.path.dirname(image_path) or "."
    return os.path.join(directory, base)


def run_batch(
    image_paths: Iterable[str],
    lang: LanguageConfig,
    out_dir: Optional[str] = None,
    on_result: Optional[Callable[[str, str], None]] = None,
) -> List[str]:
    """OCR every image in `image_paths`, writing a sibling (or `out_dir`)
    .txt file for each. Returns the list of text files written."""
    if out_dir:
        os.makedirs(out_dir, exist_ok=True)

    written = []
    for image_path in image_paths:
        text = ocr_image(image_path, lang)
        out_path = output_path_for(image_path, out_dir)
        with open(out_path, "w", encoding="utf-8") as f:
            f.write(text)
        written.append(out_path)
        if on_result:
            on_result(image_path, out_path)
    return written
