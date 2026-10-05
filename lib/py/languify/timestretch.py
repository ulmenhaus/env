"""Pitch-preserving playback speed change.

Just relabeling a WAV's sample rate (what interactive.py's speed control
used to do) changes pitch along with duration -- the same effect as
playing a tape faster or slower.

This uses pytsmod's WSOLA (Waveform Similarity Overlap-Add) implementation
-- a published, tested reference implementation of the standard
time-scale-modification algorithm (Driedger & Muller's TSM Toolbox) --
rather than a hand-rolled one. That matters here for a specific reason: a
first attempt at a hand-rolled version kept the *analysis* hop fixed and
varied the *synthesis* hop to change speed, which breaks the overlap-add
constant-gain condition (satisfied only at specific hop/window ratios,
normally 50% for a Hann window) by a different amount at every speed --
producing exactly the frequency-dependent (bass/treble) imbalance that
motivated switching to this library. pytsmod's WSOLA instead keeps the
synthesis hop fixed at 50% of the window and varies the analysis hop to
achieve the stretch, so the overlap-add reconstruction's frequency
response stays flat regardless of speed.

WSOLA (vs. a phase vocoder, the other standard option) is also the
better fit for speech specifically: phase vocoders can introduce a
smeared/"phasy" quality on transient, non-harmonic content that WSOLA's
time-domain, waveform-similarity approach avoids.
"""

import numpy as np
import pytsmod as tsm


def change_speed(pcm_bytes: bytes, speed: float) -> bytes:
    """Time-stretch 16-bit mono PCM bytes by `speed` (output duration =
    input duration / speed), preserving pitch."""
    if speed == 1.0 or not pcm_bytes:
        return pcm_bytes

    samples = np.frombuffer(pcm_bytes, dtype=np.int16).astype(np.float64)
    if len(samples) < 2048:
        return pcm_bytes

    stretched = tsm.wsola(samples, 1.0 / speed)
    stretched = np.clip(stretched, -32768, 32767)
    return stretched.astype(np.int16).tobytes()
