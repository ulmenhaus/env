"""Recognize the phonemes actually produced in a stretch of audio.

Backed by `facebook/wav2vec2-lv-60-espeak-cv-ft`, a wav2vec2 model
fine-tuned on CommonVoice with espeak-generated IPA phoneme labels. Unlike
align.py's word-level aligner, this model's own output vocabulary *is*
IPA phonemes, and as a CTC model its per-frame argmax already carries
approximate phoneme-level timing -- so recognition and "where did each
phoneme happen" come from one forward pass, no separate aligner needed.

This is meant to be called per-word (on the slice of audio that align.py
located for that word), not on a whole sentence: CTC's frame-level
timestamps are approximate, and keeping the unit small keeps the
resulting phoneme timing anchored close to where it actually happened.

Caveat worth keeping in mind when interpreting scores: this recognizer is
not perfectly transparent even on clean, native-quality audio -- in
testing against Piper-synthesized reference speech, it still produced
occasional insertions/deletions relative to the espeak G2P target for the
same text (word-boundary artifacts, coarticulation the model hasn't seen
cleanly). That means a phone-error-rate of 0 is not the expected ceiling
even for a native speaker; score against a reference baseline (see
score.py), not against a literal zero-error target.
"""

from dataclasses import dataclass
from typing import List

import numpy as np
import torch
import torchaudio

_MODEL_NAME = "facebook/wav2vec2-lv-60-espeak-cv-ft"
_TARGET_SAMPLE_RATE = 16000

_processor = None
_model = None


def _ensure_loaded() -> None:
    global _processor, _model
    if _model is not None:
        return

    # The model's own tokenizer builds a phonemizer espeak backend at
    # construction time (to support text labels), so the espeak-ng
    # library/data paths need to be registered before this import --
    # same setup g2p.py does, duplicated here since either module may be
    # imported first.
    import espeakng_loader
    from phonemizer.backend.espeak.wrapper import EspeakWrapper

    EspeakWrapper.set_library(espeakng_loader.get_library_path())
    EspeakWrapper.set_data_path(espeakng_loader.get_data_path())

    from transformers import Wav2Vec2ForCTC, Wav2Vec2Processor

    _processor = Wav2Vec2Processor.from_pretrained(_MODEL_NAME)
    _model = Wav2Vec2ForCTC.from_pretrained(_MODEL_NAME)
    _model.eval()


@dataclass
class RecognizedPhone:
    phone: str
    start: float  # seconds, relative to the audio slice passed in
    end: float


def recognize_phones(waveform, sample_rate: int) -> List[RecognizedPhone]:
    """`waveform`: 1-D float32 array/tensor of mono audio, typically one
    word's worth (see align.py). Returns the recognized phones in order
    with their approximate start/end within this slice."""
    _ensure_loaded()

    wav = torch.as_tensor(waveform, dtype=torch.float32).unsqueeze(0)
    if sample_rate != _TARGET_SAMPLE_RATE:
        wav = torchaudio.functional.resample(wav, sample_rate, _TARGET_SAMPLE_RATE)

    inputs = _processor(
        wav.squeeze(0).numpy(), sampling_rate=_TARGET_SAMPLE_RATE, return_tensors="pt"
    )
    with torch.inference_mode():
        logits = _model(inputs.input_values).logits[0]  # (n_frames, vocab)
    pred_ids = torch.argmax(logits, dim=-1).tolist()

    n_frames = len(pred_ids)
    duration = wav.size(1) / _TARGET_SAMPLE_RATE
    frame_dur = duration / n_frames

    pad_id = _processor.tokenizer.pad_token_id
    word_delim = _processor.tokenizer.word_delimiter_token

    phones: List[RecognizedPhone] = []
    current_tok = None
    current_start = 0.0
    for i, tok_id in enumerate(pred_ids + [pad_id]):  # sentinel flush at the end
        tok = (
            _processor.tokenizer.convert_ids_to_tokens([tok_id])[0]
            if tok_id != pad_id
            else None
        )
        if tok != current_tok:
            if current_tok not in (None, word_delim):
                phones.append(
                    RecognizedPhone(
                        phone=current_tok, start=current_start, end=i * frame_dur
                    )
                )
            current_tok = tok
            current_start = i * frame_dur
    return phones
