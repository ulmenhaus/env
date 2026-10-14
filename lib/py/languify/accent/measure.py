"""Acoustic measurements for one recognized phone, given its audio slice.

These exist because a categorical phoneme match (diff.py) can't see
sub-phonemic accent markers: producing the right phoneme with the wrong
vowel quality, or the right stop with the wrong voice-onset-time, both
score as a "match" in the categorical diff even though they're exactly
what a heritage/L2 speaker's accent tends to be made of. Each function
here targets one such marker; score.py's phone-class registry decides
which of them applies to a given phone.

Built on `parselmouth` (Praat bindings) for pitch/formants, plus a
straightforward energy/periodicity heuristic for VOT -- VOT in particular
is an approximation (burst = first large rise in high-band energy,
voicing onset = first reliably-pitched frame after it), not a
clinical-grade measurement, and should be treated as a researcher's quick
estimate rather than ground truth.
"""

from dataclasses import dataclass
from typing import Optional

import numpy as np
import parselmouth

from languify.accent.diff import normalize_phone

VOWELS = set("aeiouɑɔɛɪʊøœyɨəɐɯɤ")
STOPS = set("ptkbdɡqɢʔ")
FRICATIVES = set("szʃʒfvxɣħʁhθðɕʐ")


def phone_class(phone: str) -> str:
    base = normalize_phone(phone)
    if any(c in VOWELS for c in base):
        return "vowel"
    if any(c in STOPS for c in base):
        return "stop"
    if any(c in FRICATIVES for c in base):
        return "fricative"
    return "other"


@dataclass
class Duration:
    seconds: float


def measure_duration(start: float, end: float) -> Duration:
    return Duration(seconds=max(0.0, end - start))


@dataclass
class Formants:
    f1_hz: Optional[float]
    f2_hz: Optional[float]


def measure_formants(sound: parselmouth.Sound, start: float, end: float) -> Formants:
    """F1/F2 at the midpoint of [start, end] within `sound`, which should
    be at least the phone's own audio plus a little context (Praat's
    formant tracker needs some run-up) -- a single word's clip is enough."""
    formant = sound.to_formant_burg()
    mid = (start + end) / 2
    f1 = formant.get_value_at_time(1, mid)
    f2 = formant.get_value_at_time(2, mid)
    return Formants(
        f1_hz=f1 if f1 == f1 else None,  # nan check without importing math
        f2_hz=f2 if f2 == f2 else None,
    )


@dataclass
class Pitch:
    f0_hz: Optional[float]
    contour_hz: np.ndarray  # per-frame F0 across [start, end], unvoiced frames as 0


def measure_pitch(sound: parselmouth.Sound, start: float, end: float) -> Pitch:
    pitch = sound.to_pitch()
    times = [t for t in pitch.ts() if start <= t <= end]
    values = np.array([pitch.get_value_at_time(t) for t in times])
    values = np.nan_to_num(values, nan=0.0)
    voiced = values[values > 0]
    return Pitch(
        f0_hz=float(voiced.mean()) if len(voiced) else None,
        contour_hz=values,
    )


@dataclass
class SpectralCentroid:
    hz: Optional[float]


def measure_spectral_centroid(sound: parselmouth.Sound, start: float, end: float) -> SpectralCentroid:
    """Center of mass of the energy spectrum over [start, end] -- a
    standard acoustic-phonetics proxy for consonant place of articulation:
    a more front/anterior constriction (e.g. "s") concentrates energy
    higher in the spectrum than a more back/posterior one (e.g. the
    uvular "ʁ"), giving a continuous "how far back" number instead of
    just a categorical phoneme label. Most validated for genuine
    frication noise; for an approximant-like realization (e.g. a
    non-trilled resh) the signal is weaker but still present, since place
    of articulation still shapes the formant/noise energy distribution."""
    clip = sound.extract_part(from_time=start, to_time=end, preserve_times=False)
    if clip.duration < 0.005:
        return SpectralCentroid(hz=None)
    centroid = clip.to_spectrum().get_centre_of_gravity(power=2)
    return SpectralCentroid(hz=centroid if centroid == centroid else None)


@dataclass
class Voicing:
    voiced_fraction: Optional[float]  # 0-1: fraction of [start, end] Praat's pitch tracker finds periodic


def measure_voicing(sound: parselmouth.Sound, start: float, end: float) -> Voicing:
    """Fraction of [start, end] that's periodic -- a cheap, well-trodden
    proxy for voicing (vocal fold vibration) without a dedicated
    glottal-source measurement. Runs on the whole `sound`'s pitch track
    (same approach as measure_pitch) rather than a short extracted clip,
    so it isn't subject to the "too short to resolve a pitch floor" issue
    measure_vot has to work around for a tiny isolated slice."""
    pitch = sound.to_pitch()
    times = [t for t in pitch.ts() if start <= t <= end]
    if not times:
        return Voicing(voiced_fraction=None)
    values = np.array([pitch.get_value_at_time(t) for t in times])
    values = np.nan_to_num(values, nan=0.0)
    return Voicing(voiced_fraction=float(np.mean(values > 0)))


@dataclass
class VOT:
    milliseconds: Optional[float]


def measure_vot(
    sound: parselmouth.Sound, start: float, search_window: float = 0.15
) -> VOT:
    """Approximate voice onset time for a stop aligned at `start`: burst =
    first sharp rise in high-frequency energy, voicing onset = first
    frame after the burst where the signal is reliably pitched.

    Searches forward from `start` over `search_window` seconds rather
    than being confined to the stop's own aligned duration, since VOT by
    definition extends past the stop's closure into the voicing of
    whatever follows -- a word-final stop with nothing voiced after it
    will correctly come back with no VOT (`milliseconds=None`)."""
    end = min(start + search_window, sound.duration)
    clip = sound.extract_part(from_time=start, to_time=end, preserve_times=False)
    sr = clip.sampling_frequency
    samples = clip.values[0]
    if len(samples) < 32:
        return VOT(milliseconds=None)

    # High-pass via simple first difference approximates burst energy --
    # a release burst is broadband/high-frequency relative to the silence
    # or low-frequency voicing that can precede it.
    hp = np.diff(samples)
    window = max(1, int(sr * 0.002))  # ~2ms energy window
    energy = np.array(
        [np.sum(hp[i : i + window] ** 2) for i in range(0, len(hp) - window, window)]
    )
    if not len(energy) or energy.max() == 0:
        return VOT(milliseconds=None)
    burst_frame = int(np.argmax(energy > 0.3 * energy.max()))
    burst_time = burst_frame * window / sr

    remaining = clip.duration - burst_time
    if remaining < 3 / 500:  # too short to resolve pitch at any sane floor
        return VOT(milliseconds=None)
    # Praat's pitch analysis window needs ~3 periods to fit in the clip,
    # so a short remaining span needs a correspondingly higher floor than
    # the standard 75Hz default (which assumes a multi-period window).
    pitch_floor = max(75.0, min(500.0, 3 / remaining))
    pitch = clip.to_pitch(pitch_floor=pitch_floor)
    voiced_times = [
        t for t in pitch.ts() if t >= burst_time and pitch.get_value_at_time(t) > 0
    ]
    if not voiced_times:
        return VOT(milliseconds=None)
    voicing_onset = voiced_times[0]
    return VOT(milliseconds=(voicing_onset - burst_time) * 1000)
