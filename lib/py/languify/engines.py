"""TTS engine adapters.

Farsi and Hebrew need two different libraries -- Piper has no Hebrew
voice, and israwave is Hebrew-only -- so each gets a thin adapter that
normalizes its library's native output to the same shape: raw 16-bit
mono PCM bytes at `self.sample_rate`. Everything above this module (the
combine-mode writer and the interactive reader) talks to that one shape
and never imports piper/israwave directly.

Both adapters call into their library's Python API in-process (no
`piper`/`israwave` CLI subprocess per call), so a model loads once per
`languify tts` invocation no matter how many sentences/files it ends up
synthesizing.
"""

import os
from typing import Any, Dict

from languify.config import LanguageConfig


def _require_path(path, label):
    if not path:
        raise FileNotFoundError(
            f"{label} is not set in ~/.languify.json. See languify_setup.txt."
        )
    expanded = os.path.expanduser(str(path))
    if not os.path.exists(expanded):
        raise FileNotFoundError(
            f"{label} not found at '{expanded}'. See languify_setup.txt for "
            "where to download the required model files."
        )
    return expanded


class TTSEngine:
    sample_rate: int

    def synthesize(self, text: str) -> bytes:
        """Return 16-bit mono PCM bytes for `text`."""
        raise NotImplementedError


class PiperEngine(TTSEngine):
    """Farsi (and any other Piper voice) via the piper-tts Python package.

    piper-tts changed maintainers (now OHF-Voice/piper1-gpl) and its
    Python API along with it: the old `synthesize_stream_raw(text,
    length_scale=..., ...)` generator is gone. The current API configures
    synthesis via a `SynthesisConfig` object and `voice.synthesize(text,
    syn_config=...)` yields `AudioChunk` objects (one per internal
    sentence) instead of raw byte chunks -- this also dropped the old
    `sentence_silence` parameter, so any configured silence between
    sentences is now inserted here instead, between those chunks.
    """

    def __init__(self, options: Dict[str, Any]):
        try:
            from piper import PiperVoice, SynthesisConfig
        except ImportError as exc:
            raise ImportError(
                "piper-tts is required for this language's TTS -- see "
                "languify_setup.txt"
            ) from exc

        model_path = _require_path(options.get("model_path"), "tts.model_path")
        config_path = options.get("config_path")
        if config_path:
            config_path = _require_path(config_path, "tts.config_path")

        self.voice = PiperVoice.load(model_path, config_path=config_path)
        self.sample_rate = self.voice.config.sample_rate
        self.sentence_silence = options.get("sentence_silence", 0.0)
        self.syn_config = SynthesisConfig(
            length_scale=options.get("length_scale"),
            noise_scale=options.get("noise_scale"),
            noise_w_scale=options.get("noise_w"),
        )

    def synthesize(self, text: str) -> bytes:
        silence = bytes(int(self.sentence_silence * self.sample_rate) * 2)
        chunks = [
            chunk.audio_int16_bytes
            for chunk in self.voice.synthesize(text, syn_config=self.syn_config)
        ]
        return silence.join(chunks) if silence else b"".join(chunks)


class IsrawaveEngine(TTSEngine):
    """Hebrew via israwave (IsraWave speech model + Nakdimon niqqud model)."""

    def __init__(self, options: Dict[str, Any]):
        try:
            import numpy as np
            from israwave import IsraWave
            from israwave.segment import SegmentExtractor
            from nakdimon_ort import Nakdimon
        except ImportError as exc:
            raise ImportError(
                "israwave (and its nakdimon_ort dependency) is required for "
                "this language's TTS -- see languify_setup.txt"
            ) from exc

        self._np = np
        model_path = _require_path(options.get("model_path"), "tts.model_path")
        espeak_data_path = _require_path(
            options.get("espeak_data_path"), "tts.espeak_data_path"
        )
        nakdimon_path = _require_path(options.get("nakdimon_path"), "tts.nakdimon_path")

        self.model = IsraWave(model_path, espeak_data_path)
        self.nakdimon = Nakdimon(nakdimon_path)
        self.segmenter = SegmentExtractor()
        self.sample_rate = self.model.sample_rate

    def synthesize(self, text: str) -> bytes:
        np = self._np
        vocalized = self.nakdimon.compute(text)

        waveforms = []
        for segment in self.segmenter.extract_segments(vocalized):
            waveform = self.model.create(segment.text)
            waveforms.append(waveform.samples)
            waveforms.append(segment.create_pause(waveform.sample_rate))

        if not waveforms:
            return b""

        final = np.concatenate(waveforms)
        if final.dtype.kind == "f":
            # israwave's waveforms are float32 in [-1, 1], same convention
            # sounddevice expects in its own examples/play.py.
            final = np.clip(final, -1.0, 1.0)
            final = (final * 32767).astype(np.int16)
        elif final.dtype != np.int16:
            final = final.astype(np.int16)
        return final.tobytes()


_ENGINES = {
    "piper": PiperEngine,
    "israwave": IsrawaveEngine,
}


def build_engine(lang: LanguageConfig) -> TTSEngine:
    engine_name = lang.tts.engine
    if engine_name not in _ENGINES:
        raise ValueError(
            f"Unknown TTS engine '{engine_name}' for language '{lang.name}'. "
            f"Supported: {sorted(_ENGINES)}"
        )
    return _ENGINES[engine_name](lang.tts.options)
