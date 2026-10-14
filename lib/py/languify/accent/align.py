"""Word-level forced alignment: find when each word of a known transcript
is spoken in an audio recording.

Backed by Meta's MMS forced-alignment model (`torchaudio.pipelines.MMS_FA`),
which works across 1,100+ languages -- including Farsi and Hebrew, neither
of which has a pretrained Montreal Forced Aligner acoustic model. MMS_FA's
acoustic model was trained on romanized (Latin-script) text via `uroman`,
not on each language's native phoneme set, so this module only produces
WORD-level timestamps, not phoneme-level ones: `uroman`'s transliteration
of Farsi/Hebrew script is a crude letter transcription that drops short
vowels, so it's good enough to anchor word boundaries (which are mostly
load-bearing on the acoustic signal itself) but not to resolve phones
within a word.

Phoneme-level timing comes from a different model entirely (see
recognize.py) run on each word's audio slice -- which is also why this
split of labor exists: two models, each used for the granularity it's
actually trained for.
"""

from dataclasses import dataclass
from typing import List

import torch
import torchaudio
import uroman

from languify.accent.normalize import spell_out_if_numeral, strip_punctuation

_bundle = torchaudio.pipelines.MMS_FA
_model = None
_tokenizer = None
_aligner = None
_romanizer = None
_dictionary = None


def _ensure_loaded() -> None:
    global _model, _tokenizer, _aligner, _romanizer, _dictionary
    if _model is not None:
        return
    _model = _bundle.get_model(with_star=False)
    _model.eval()
    _tokenizer = _bundle.get_tokenizer()
    _aligner = _bundle.get_aligner()
    _romanizer = uroman.Uroman()
    # The tokenizer does plain dict[c] lookups with no fallback (see
    # torchaudio.pipelines._wav2vec2.aligner.Tokenizer.__call__), so an
    # unsupported character raises a bare KeyError -- checked for here
    # instead, to fail with a word-level, actionable ValueError rather
    # than crashing the caller. The dictionary is just a-z plus a few
    # punctuation marks: no digits, so numerals written as digits
    # (uroman romanizes e.g. Persian "۲" straight to ASCII "2") are the
    # most likely way to hit this in Farsi/Hebrew text.
    _dictionary = _bundle.get_dict()


@dataclass
class WordSpan:
    word: str
    start: float  # seconds
    end: float
    confidence: float  # mean per-token alignment score, roughly 0-1


def align_words(
    waveform, sample_rate: int, words: List[str], language: str
) -> List[WordSpan]:
    """`waveform`: 1-D float32 array/tensor of mono audio at `sample_rate`.
    Returns one WordSpan per entry in `words`, in order (each `WordSpan.word`
    keeps the original text, punctuation included -- only the text fed to
    uroman is normalized, same reasoning as g2p.py; see normalize.py)."""
    _ensure_loaded()

    wav = torch.as_tensor(waveform, dtype=torch.float32).unsqueeze(0)
    if sample_rate != _bundle.sample_rate:
        wav = torchaudio.functional.resample(wav, sample_rate, _bundle.sample_rate)

    normalized = [
        spell_out_if_numeral(strip_punctuation(w), language) for w in words
    ]
    romanized = [_romanizer.romanize_string(w).strip().lower() for w in normalized]
    empty = [i for i, r in enumerate(romanized) if not r]
    if empty:
        raise ValueError(
            f"uroman produced an empty romanization for word(s) at index "
            f"{empty}: {[words[i] for i in empty]!r}"
        )
    unsupported = {
        words[i]: sorted(set(r) - _dictionary.keys())
        for i, r in enumerate(romanized)
        if set(r) - _dictionary.keys()
    }
    if unsupported:
        raise ValueError(
            f"word(s) romanize to characters the forced aligner doesn't "
            f"support (likely digits -- try spelling numbers out as "
            f"words): {unsupported}"
        )

    with torch.inference_mode():
        emission, _ = _model(wav)
    tokens = _tokenizer(romanized)
    token_spans = _aligner(emission[0], tokens)

    # Frames -> seconds: emission has one frame per ~20ms of audio (the
    # model's fixed stride), so this ratio holds regardless of utterance
    # length rather than being a hardcoded constant.
    ratio = wav.size(1) / emission.size(1) / _bundle.sample_rate

    spans = []
    for word, word_token_spans in zip(words, token_spans):
        start = word_token_spans[0].start * ratio
        end = word_token_spans[-1].end * ratio
        confidence = sum(s.score for s in word_token_spans) / len(word_token_spans)
        spans.append(WordSpan(word=word, start=start, end=end, confidence=confidence))
    return spans
