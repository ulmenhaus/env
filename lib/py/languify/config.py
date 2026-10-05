"""Load and validate the languify configuration file.

Model paths and per-engine synthesis parameters live in an external JSON
config (~/.languify.json by default) rather than in code, so a voice or
model version can be swapped out by editing that file instead of this
package. See languify_setup.txt (repo root) for how to produce the file
and where the models it points at should be downloaded to.
"""

import json
import os
from dataclasses import dataclass, field
from typing import Any, Dict, Optional

DEFAULT_CONFIG_PATH = os.path.expanduser("~/.languify.json")

# Accepted spellings for each supported language, mapped to the canonical
# key used both in the config file and throughout this package.
LANGUAGE_ALIASES = {
    "farsi": "farsi", "fa": "farsi", "fas": "farsi", "persian": "farsi", "fa_ir": "farsi",
    "hebrew": "hebrew", "he": "hebrew", "heb": "hebrew", "he_il": "hebrew",
}

# Properties of each language's writing system, not of the configured
# model -- so they live here rather than in ~/.languify.json.
# RTL: both Hebrew and Farsi are right-to-left scripts.
RTL = {"farsi": True, "hebrew": True}
# NEEDS_RESHAPING: whether letters need shaping (joining) before display --
# Arabic-script Farsi does, Hebrew does not.
NEEDS_RESHAPING = {"farsi": True, "hebrew": False}


def canonical_language(name: str) -> str:
    key = name.strip().lower()
    if key not in LANGUAGE_ALIASES:
        supported = sorted(set(LANGUAGE_ALIASES.values()))
        raise ValueError(f"Unknown language '{name}'. Supported: {supported}")
    return LANGUAGE_ALIASES[key]


@dataclass
class OCRConfig:
    engine: str
    lang_code: str


@dataclass
class TTSConfig:
    engine: str
    options: Dict[str, Any] = field(default_factory=dict)


@dataclass
class LanguageConfig:
    name: str
    rtl: bool
    needs_reshaping: bool
    ocr: OCRConfig
    tts: TTSConfig


def _expand(value):
    if isinstance(value, str):
        return os.path.expanduser(value)
    return value


def load_config(path: Optional[str] = None) -> Dict[str, LanguageConfig]:
    """Parse the full config file into a dict keyed by canonical language."""
    path = path or DEFAULT_CONFIG_PATH
    if not os.path.exists(path):
        raise FileNotFoundError(
            f"languify config not found at {path}.\n"
            "Copy languify.json (repo root) to that path, or pass --config "
            "explicitly. See languify_setup.txt for model setup."
        )
    with open(path, encoding="utf-8") as f:
        raw = json.load(f)

    languages: Dict[str, LanguageConfig] = {}
    for name, entry in raw.items():
        ocr_raw = entry.get("ocr", {})
        tts_raw = entry.get("tts", {})
        tts_options = {k: _expand(v) for k, v in tts_raw.items() if k != "engine"}
        languages[name] = LanguageConfig(
            name=name,
            rtl=RTL.get(name, False),
            needs_reshaping=NEEDS_RESHAPING.get(name, False),
            ocr=OCRConfig(
                engine=ocr_raw.get("engine", "tesseract"),
                lang_code=ocr_raw.get("lang_code", name),
            ),
            tts=TTSConfig(engine=tts_raw.get("engine"), options=tts_options),
        )
    return languages


def get_language_config(language: str, path: Optional[str] = None) -> LanguageConfig:
    canon = canonical_language(language)
    languages = load_config(path)
    if canon not in languages:
        raise KeyError(
            f"No configuration for language '{canon}' in config file. "
            f"Available: {sorted(languages)}"
        )
    return languages[canon]
