"""Orchestrates one practice pass: given a sentence and a recording of it,
produce a per-word, per-phone report combining the categorical phoneme
diff (diff.py) with the acoustic measurements (measure.py) that diff
alone can't see.

This is deliberately UI-less -- it hands back plain dataclasses so both
a simple CLI printout and (eventually) the curses TUIs can render the
same underlying report differently.
"""

from dataclasses import dataclass
from typing import List, Optional

import parselmouth

from languify.accent import align, measure, recognize
from languify.accent.diff import Op, align_phonemes, normalize_phone
from languify.accent.g2p import WordPhonemes, phonemize_words

WORD_PAD_SECONDS = 0.05


@dataclass
class PhoneReport:
    op: Op
    target: Optional[str]
    recognized: Optional[str]
    duration_ms: Optional[float]  # of the recognized phone, if there is one
    formants: Optional[measure.Formants] = None
    vot_ms: Optional[float] = None
    voicing: Optional[measure.Voicing] = None  # every phone class: devoicing shows up anywhere
    centroid_hz: Optional[float] = None  # fricatives only: place-of-articulation proxy


@dataclass
class WordReport:
    word: str
    start: float
    end: float
    align_confidence: float
    phones: List[PhoneReport]
    phone_error_rate: float
    # Index into the target-anchored phone list (see interactive.py's
    # _target_anchored) of the primary-stressed phone, if espeak marked
    # one -- None for unstressed/monosyllabic words. stress_ratio is this
    # word's own stressed-phone duration divided by the mean duration of
    # its other phones: a within-word, self-normalized number, so it's
    # comparable across speakers/recording levels the way a raw duration
    # or dB value wouldn't be (same register-confound lesson as F0).
    stressed_target_index: Optional[int] = None
    stress_ratio: Optional[float] = None


def _score_word(
    sound: parselmouth.Sound,
    audio: "numpy.ndarray",
    sample_rate: int,
    span: align.WordSpan,
    target: WordPhonemes,
) -> WordReport:
    i0 = max(0, int((span.start - WORD_PAD_SECONDS) * sample_rate))
    i1 = min(len(audio), int((span.end + WORD_PAD_SECONDS) * sample_rate))
    clip_offset = i0 / sample_rate
    recognized = recognize.recognize_phones(audio[i0:i1], sample_rate)

    target_norm = [normalize_phone(p) for p in target.phonemes]
    recognized_norm = [normalize_phone(p.phone) for p in recognized]
    aligned = align_phonemes(target_norm, recognized_norm)

    phone_reports = []
    for a in aligned:
        duration_ms = None
        formants = None
        vot_ms = None
        voicing = None
        centroid_hz = None
        if a.recognized_index is not None:
            rp = recognized[a.recognized_index]
            start_abs = clip_offset + rp.start
            end_abs = clip_offset + rp.end
            duration_ms = (end_abs - start_abs) * 1000
            voicing = measure.measure_voicing(sound, start_abs, end_abs)
            cls = measure.phone_class(a.recognized)
            if cls == "vowel":
                formants = measure.measure_formants(sound, start_abs, end_abs)
            elif cls == "stop":
                vot_ms = measure.measure_vot(sound, start_abs).milliseconds
            elif cls == "fricative":
                centroid_hz = measure.measure_spectral_centroid(sound, start_abs, end_abs).hz
        phone_reports.append(
            PhoneReport(
                op=a.op,
                target=a.target,
                recognized=a.recognized,
                duration_ms=duration_ms,
                formants=formants,
                vot_ms=vot_ms,
                voicing=voicing,
                centroid_hz=centroid_hz,
            )
        )

    errors = sum(1 for p in phone_reports if p.op in (Op.SUBSTITUTE, Op.DELETE))
    per = errors / len(target.phonemes) if target.phonemes else 0.0

    # Stress: espeak marks the primary-stressed phone with a leading "ˈ"
    # in the *raw* (pre-normalize_phone) target string -- normalize_phone
    # strips it for the categorical diff, since the recognizer never
    # outputs stress marks, but the raw form still has it here.
    stressed_target_index = next(
        (i for i, p in enumerate(target.phonemes) if p.startswith("ˈ")), None
    )
    stress_ratio = None
    if stressed_target_index is not None:
        anchored = [p for p in phone_reports if p.op != Op.INSERT]
        if stressed_target_index < len(anchored):
            stressed_dur = anchored[stressed_target_index].duration_ms
            other_durs = [
                p.duration_ms for i, p in enumerate(anchored)
                if i != stressed_target_index and p.duration_ms is not None
            ]
            if stressed_dur is not None and other_durs:
                stress_ratio = stressed_dur / (sum(other_durs) / len(other_durs))

    return WordReport(
        word=span.word,
        start=span.start,
        end=span.end,
        align_confidence=span.confidence,
        phones=phone_reports,
        phone_error_rate=per,
        stressed_target_index=stressed_target_index,
        stress_ratio=stress_ratio,
    )


def score_sentence(
    audio,
    sample_rate: int,
    words: List[str],
    language: str,
    nakdimon_path: Optional[str] = None,
) -> List[WordReport]:
    """`audio`: 1-D float32 array/tensor of the full sentence recording.
    `words`: the sentence's words in order, exactly as written (this
    drives both G2P and what align.py looks for in the audio)."""
    targets = phonemize_words(words, language, nakdimon_path=nakdimon_path)
    spans = align.align_words(audio, sample_rate, words, language)

    import numpy as np

    audio_np = np.asarray(audio, dtype=np.float32)
    sound = parselmouth.Sound(audio_np, sampling_frequency=sample_rate)

    return [
        _score_word(sound, audio_np, sample_rate, span, target)
        for span, target in zip(spans, targets)
    ]
