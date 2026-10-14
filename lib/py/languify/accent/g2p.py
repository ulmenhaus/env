"""Grapheme-to-phoneme (target pronunciation) for Farsi and Hebrew.

Backed by espeak-ng via `phonemizer` + `espeakng-loader` (no system
espeak-ng install needed -- the loader bundles the shared library and its
data files as a pip wheel).

Hebrew text is normally unvocalized (no niqqud), so espeak-ng's Hebrew
dictionary can't resolve its vowels without help -- the same problem
languify's IsrawaveEngine already solves for TTS by running text through
the Nakdimon niqqud-restoration model first (see engines.py). This module
does the same before phonemizing Hebrew.

Phonemization runs per word rather than on the whole sentence at once, so
each word's phoneme sequence stays independently addressable -- this is
what lets the practice UI score and highlight one word, or step through
one phoneme within a word, at a time. The cost is losing cross-word
sandhi (assimilation across a word boundary), which is an acceptable
simplification for practice purposes.
"""

from dataclasses import dataclass
from typing import List, Optional

import espeakng_loader
from phonemizer import phonemize
from phonemizer.backend.espeak.wrapper import EspeakWrapper
from phonemizer.separator import Separator

from languify.accent.normalize import strip_punctuation

_ESPEAK_LANGUAGE = {"farsi": "fa", "hebrew": "he"}
_SEPARATOR = Separator(phone=" ", word="|", syllable=None)

_library_ready = False


def _ensure_espeak_library() -> None:
    global _library_ready
    if _library_ready:
        return
    EspeakWrapper.set_library(espeakng_loader.get_library_path())
    EspeakWrapper.set_data_path(espeakng_loader.get_data_path())
    _library_ready = True


@dataclass
class WordPhonemes:
    word: str
    phonemes: List[str]  # IPA phone tokens, in pronunciation order


def _vocalize_hebrew(words: List[str], nakdimon_path: str) -> List[str]:
    """Restore niqqud to unvocalized Hebrew words via Nakdimon, vocalizing
    the whole sentence at once (not word-by-word) so the model has
    context, then re-splitting on whitespace to recover word boundaries."""
    from nakdimon_ort import Nakdimon

    nakdimon = Nakdimon(nakdimon_path)
    joined = nakdimon.compute(" ".join(words))
    vocalized = joined.split(" ")
    if len(vocalized) != len(words):
        raise RuntimeError(
            f"Nakdimon vocalization changed word count ({len(words)} -> "
            f"{len(vocalized)}); word/phoneme alignment would be wrong."
        )
    return vocalized


def phonemize_words(
    words: List[str], language: str, nakdimon_path: Optional[str] = None
) -> List[WordPhonemes]:
    """Return the target (native-pronunciation) phoneme sequence for each
    word, via espeak-ng's grapheme-to-phoneme rules for that language.

    `WordPhonemes.word` keeps each word exactly as given (e.g. with
    attached punctuation) for display -- only the text handed to espeak
    is stripped, since a leading/trailing punctuation mark can corrupt
    espeak's phonemization of the whole word (see normalize.py)."""
    if language not in _ESPEAK_LANGUAGE:
        raise ValueError(f"Unsupported language '{language}' for G2P")
    _ensure_espeak_library()

    stripped = [strip_punctuation(w) for w in words]

    if language == "hebrew":
        if not nakdimon_path:
            raise ValueError(
                "hebrew G2P requires nakdimon_path (niqqud restoration) -- "
                "see tts.nakdimon_path in ~/.languify.json"
            )
        source_words = _vocalize_hebrew(stripped, nakdimon_path)
    else:
        source_words = stripped

    raw = phonemize(
        source_words,
        language=_ESPEAK_LANGUAGE[language],
        backend="espeak",
        with_stress=True,
        separator=_SEPARATOR,
        strip=True,
    )
    return [
        # espeak-ng leaks a bare digit phoneme for some consonants (e.g.
        # Farsi "ق" always comes back as "q 1", confirmed by testing every
        # word containing it) -- real IPA output is never a plain digit,
        # so dropping those tokens is safe rather than scoring against a
        # phoneme that isn't actually one.
        WordPhonemes(
            word=word,
            phonemes=[p for p in phones.split(" ") if not p.isdigit()] if phones else [],
        )
        for word, phones in zip(words, raw)
    ]
