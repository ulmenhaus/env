"""Tiny WAV read/write helpers shared by the combine-mode and interactive
TTS paths. Both TTS engines (see engines.py) are normalized to hand back
plain 16-bit mono PCM bytes plus a sample rate, so this is the only place
that needs to know about the `wave` module.
"""

import wave


def write_wav(path: str, pcm_bytes: bytes, sample_rate: int) -> str:
    with wave.open(path, "wb") as wf:
        wf.setnchannels(1)
        wf.setsampwidth(2)  # 16-bit
        wf.setframerate(sample_rate)
        wf.writeframes(pcm_bytes)
    return path
