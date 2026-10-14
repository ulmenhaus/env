"""Curses reader for accent practice: shows one sentence at a time, lets
you record yourself saying it, and shows the per-word score once scoring
finishes.

Controls:
  r           start recording -- press again to stop and score
  p           play the reference pronunciation (Piper/israwave TTS) --
              press again to stop; unavailable if the mic is recording
  up/down     select a word in the results list
  z           zoom into the selected word: step phone by phone (left/
              right) and see your measurements against the reference's
              own (z or Esc to zoom back out)
  left/right  previous/next sentence (zoom: previous/next phone)
  enter       re-paste any clipboard ("-") source(s)
  ?           toggle a help overlay explaining each measurement (duration,
              voiced%, F1, F2, VOT) and how to correct it
  q           quit

Unlike languify.interactive's TTS reader (which pages several sentences
per screen, since it only needs room for the text itself), this shows
one sentence per screen: scoring a sentence needs a per-word results
list underneath it, and that needs the vertical room multi-sentence
paging would otherwise use.

Scoring runs synchronously on the main thread the moment you stop
recording -- there is no threading here yet, so the UI will freeze
(un-redrawable, key presses queued but not handled) for however long
alignment + recognition + measurement take on that sentence. A "Scoring
..." status is drawn immediately before that blocking call so you at
least see it start.
"""

import curses
import locale
import os
import shutil
import sys
import tempfile
from typing import List, Optional

import numpy as np

from languify.accent import measure
from languify.accent.capture import Recorder
from languify.accent.diff import Op
from languify.accent.pipeline import PhoneReport, WordReport, score_sentence
from languify.audio_io import write_wav
from languify.bidi_render import to_visual
from languify.config import LanguageConfig
from languify.engines import build_engine
from languify.player import Player
from languify.textutil import split_sentences
from languify.tts import CLIPBOARD_MARKER, read_text

MARGIN = 2
HEADER_ROWS = 3
FOOTER_ROWS = 2
POLL_MS = 100
MAX_RECORD_SECONDS = 20.0

# Score buckets for a word's phone_error_rate -> color pair index (set up
# in _init_colors). Picked to be forgiving: a single substitution in a
# short word shouldn't read as "red."
GOOD_THRESHOLD = 0.15
OK_THRESHOLD = 0.4

COLOR_GOOD = 1
COLOR_OK = 2
COLOR_BAD = 3
COLOR_DIM = 4

# Closeness-to-reference thresholds for the zoom view, one pair per
# measurement kind since they're on different scales. VOT is compared as
# an absolute ms difference (the literature treats VOT categorically --
# short/long-lag -- rather than proportionally); formants and duration
# are compared relative to the reference value. All of these are
# heuristic judgment calls, not validated thresholds -- treat the zoom
# view's coloring as directional, not as ground truth.
VOT_GOOD_MS, VOT_OK_MS = 15.0, 30.0
FORMANT_GOOD_REL, FORMANT_OK_REL = 0.10, 0.20
DURATION_GOOD_REL, DURATION_OK_REL = 0.25, 0.50
# Voicing is displayed as a percentage (the row values are already *100
# of the underlying 0-1 fraction), so compared as an absolute
# percentage-point gap rather than relative (relative-of-a-fraction gets
# weird near zero) -- these thresholds are in that same 0-100 scale.
VOICING_GOOD_PP, VOICING_OK_PP = 15.0, 30.0

HELP_LINES = [
    "duration   How long you held the sound, vs. the reference's timing for",
    "           the same phone. Too short/long usually means you're rushing",
    "           or dragging that sound -- try matching the reference's pace",
    "           (press p to hear it). Note: very short sounds in fast speech",
    "           can hit the recognizer's ~20ms timing resolution floor, where",
    "           real differences stop being distinguishable -- treat exact",
    "           duration numbers as less trustworthy the shorter they are.",
    "",
    "voiced %   Fraction of the sound where your vocal folds were vibrating.",
    "           A sound that should be voiced (b, d, z, m, n...) showing low",
    "           voiced% means you're devoicing it -- common when the",
    "           equivalent sound in your other language is voiceless.",
    "           Fix: feel for a buzz in your throat/chest while producing it.",
    "",
    "F1         Vowel tongue height / mouth openness. Lower F1 = tongue",
    "           higher, mouth more closed; higher F1 = tongue lower, mouth",
    "           more open. If yours reads lower than the reference, open",
    "           your mouth / lower your tongue more for that vowel (and",
    "           vice versa if yours reads higher).",
    "",
    "F2         Vowel tongue front/back position. Higher F2 = tongue",
    "           fronted; lower F2 = tongue backed. Move your tongue forward",
    "           or backward to shift toward the reference value.",
    "",
    "VOT        Voice onset time: the gap between a stop's release and",
    "           voicing starting. Much longer than the reference usually",
    "           means you're aspirating it (a puff of air) more than the",
    "           target language does -- aim for a crisper, less-aspirated",
    "           release. Much shorter can mean the opposite: too little",
    "           release noise before voicing kicks in.",
    "",
    "centroid   Fricative place of articulation (s, sh, the Hebrew resh...):",
    "           where the consonant's noise energy concentrates in the",
    "           spectrum. Higher = produced further forward in the mouth;",
    "           lower = further back in the throat. If yours reads much",
    "           higher than the reference for a back consonant like resh,",
    "           you're likely substituting a more forward sound (e.g. an",
    "           English-style r) -- try pulling the constriction further",
    "           back, toward the uvula/throat.",
    "",
    "stress     Shown only on the word's primary-stressed syllable (marked",
    "           with ˈ in the target strip): how much longer that syllable",
    "           is than the word's other syllables, as a percentage (100% =",
    "           same length as the rest, 180% = 80% longer). If yours reads",
    "           much lower than the reference's, you're under-emphasizing",
    "           the stressed syllable -- try lengthening/leaning into it",
    "           more, the way the reference does.",
    "",
    "colors     green = close to the reference, yellow = noticeably off,",
    "           red = well off. delta = you minus reference, so a positive",
    "           number means your value reads higher than the reference's.",
]


def _init_colors() -> bool:
    if not curses.has_colors():
        return False
    curses.start_color()
    try:
        curses.use_default_colors()
        bg = -1
    except curses.error:
        bg = curses.COLOR_BLACK
    curses.init_pair(COLOR_GOOD, curses.COLOR_GREEN, bg)
    curses.init_pair(COLOR_OK, curses.COLOR_YELLOW, bg)
    curses.init_pair(COLOR_BAD, curses.COLOR_RED, bg)
    curses.init_pair(COLOR_DIM, curses.COLOR_CYAN, bg)
    return True


def _score_color(per: float, has_color: bool) -> int:
    if not has_color:
        return curses.A_BOLD if per > OK_THRESHOLD else curses.A_NORMAL
    if per <= GOOD_THRESHOLD:
        return curses.color_pair(COLOR_GOOD)
    if per <= OK_THRESHOLD:
        return curses.color_pair(COLOR_OK)
    return curses.color_pair(COLOR_BAD)


def _closeness_color(you: float, reference: float, kind: str, has_color: bool) -> int:
    if kind == "vot":
        diff, good, ok = abs(you - reference), VOT_GOOD_MS, VOT_OK_MS
    elif kind == "voicing":
        diff, good, ok = abs(you - reference), VOICING_GOOD_PP, VOICING_OK_PP
    elif kind == "formant":
        diff = abs(you - reference) / reference if reference else float("inf")
        good, ok = FORMANT_GOOD_REL, FORMANT_OK_REL
    else:  # duration
        diff = abs(you - reference) / reference if reference else float("inf")
        good, ok = DURATION_GOOD_REL, DURATION_OK_REL
    if not has_color:
        return curses.A_BOLD if diff > ok else curses.A_NORMAL
    if diff <= good:
        return curses.color_pair(COLOR_GOOD)
    if diff <= ok:
        return curses.color_pair(COLOR_OK)
    return curses.color_pair(COLOR_BAD)


def _target_anchored(phones: List[PhoneReport]) -> List[PhoneReport]:
    """`phones` in alignment order includes INSERT entries (an extra
    recognized phone with no corresponding target phone), which breaks a
    simple index-for-index comparison between two independently-aligned
    phone lists (yours and the reference's) -- each may have inserted a
    different number of extras in different places. Dropping INSERTs
    leaves exactly one entry per target phone, in target order, for both
    lists alike, so position i in one lines up with position i in the
    other."""
    return [p for p in phones if p.op != Op.INSERT]


class Session:
    def __init__(self, stdscr, text_paths: List[str], lang: LanguageConfig, nakdimon_path: Optional[str]):
        self.stdscr = stdscr
        self.text_paths = text_paths
        self.lang = lang
        self.nakdimon_path = nakdimon_path
        self.has_color = _init_colors()

        self.recorder = Recorder()
        self.status = "idle"  # idle | recording | scoring | scored | error
        self.message = ""
        self.reports_by_sentence: dict[int, List[WordReport]] = {}
        self.reference_reports_by_sentence: dict[int, List[WordReport]] = {}
        self.selected_word = 0
        self.zoom = False
        self.zoom_phone = 0
        self.help = False
        self.cache_dir = tempfile.mkdtemp(prefix="languify-accent-")

        self.player = Player()
        self.playing = False
        self.reference_pcm_cache: dict[int, bytes] = {}
        self.engine = None
        self.engine_error: Optional[str] = None
        try:
            self.engine = build_engine(lang)
        except (ImportError, FileNotFoundError, ValueError, KeyError) as exc:
            self.engine_error = str(exc)

        self.current = 0
        self.wrapped: List[str] = []

        self.source_sentences: List[List[str]] = [
            split_sentences(read_text(path)) for path in text_paths
        ]
        self.sentences: List[str] = []
        self._rebuild_sentences()
        self.layout()

    def _rebuild_sentences(self) -> None:
        self.sentences = [s for group in self.source_sentences for s in group]
        if self.sentences:
            self.current = min(self.current, len(self.sentences) - 1)

    def refresh_clipboard(self) -> None:
        indices = [i for i, p in enumerate(self.text_paths) if p == CLIPBOARD_MARKER]
        if not indices:
            return
        try:
            for i in indices:
                self.source_sentences[i] = split_sentences(read_text(CLIPBOARD_MARKER))
        except RuntimeError as exc:
            self.message = str(exc)
            return
        jump_to = sum(len(group) for group in self.source_sentences[: indices[0]])
        self._rebuild_sentences()
        self.reports_by_sentence.clear()
        self.reference_reports_by_sentence.clear()
        self.selected_word = 0
        self.zoom = False
        self.current = min(jump_to, max(len(self.sentences) - 1, 0))
        self.message = ""
        self.layout()

    # -- layout / drawing ---------------------------------------------------

    def layout(self) -> None:
        rows, cols = self.stdscr.getmaxyx()
        avail_cols = max(cols - 2 * MARGIN, 10)
        import textwrap

        text = self.sentences[self.current] if self.sentences else ""
        self.wrapped = textwrap.wrap(text, width=avail_cols) or [""]

    def draw(self) -> None:
        if self.help:
            self._draw_help()
            return
        if self.zoom:
            self._draw_zoom()
            return
        self.stdscr.erase()
        rows, cols = self.stdscr.getmaxyx()

        header = f"languify accent -- {self.lang.name}    Sentence {self.current + 1}/{len(self.sentences)}"
        self._addstr(0, 0, header, cols, curses.A_BOLD)

        status_text = {
            "idle": "idle",
            "recording": "RECORDING -- press r to stop",
            "scoring": "scoring...",
            "scored": "scored",
            "error": "error",
        }[self.status]
        info = f"[{status_text}]"
        if self.playing:
            info += "  [playing reference]"
        if self.message:
            info += f"  -- {self.message}"
        attr = curses.A_NORMAL
        if self.status == "error":
            attr = curses.color_pair(COLOR_BAD) if self.has_color else curses.A_BOLD
        elif self.status == "recording":
            attr = (curses.color_pair(COLOR_OK) if self.has_color else 0) | curses.A_BOLD
        self._addstr(1, 0, info, cols, attr)

        row = HEADER_ROWS
        if not self.sentences:
            self._addstr(row, MARGIN, "(no text loaded)", cols, curses.A_DIM)
            self.stdscr.refresh()
            return

        for line in self.wrapped:
            if row >= rows - FOOTER_ROWS:
                break
            visual = to_visual(line, self.lang.needs_reshaping)
            text = visual[: cols - 2 * MARGIN]
            x = MARGIN
            if self.lang.rtl:
                x = max(MARGIN, cols - MARGIN - len(text))
            self._addstr(row, x, text, cols, curses.A_BOLD)
            row += 1
        row += 1

        report = self.reports_by_sentence.get(self.current)
        if report is not None and row < rows - FOOTER_ROWS:
            self._addstr(
                row, MARGIN,
                "Results (word  %correct  [.=match x=substituted -=missing +=extra]):",
                cols, curses.A_DIM,
            )
            row += 1
            for i, wr in enumerate(report):
                if row >= rows - FOOTER_ROWS:
                    self._addstr(row - 1, MARGIN, "...", cols, curses.A_DIM)
                    break
                pct = int(round((1 - wr.phone_error_rate) * 100))
                word_visual = to_visual(wr.word, self.lang.needs_reshaping)
                ops = "".join(
                    {
                        Op.MATCH: ".",
                        Op.SUBSTITUTE: "x",
                        Op.DELETE: "-",
                        Op.INSERT: "+",
                    }[p.op]
                    for p in wr.phones
                )
                marker = ">" if i == self.selected_word else " "
                line = f"{marker} {word_visual}  {pct:3d}%  [{ops}]"
                attr = _score_color(wr.phone_error_rate, self.has_color)
                if i == self.selected_word:
                    attr |= curses.A_BOLD
                self._addstr(row, MARGIN, line, cols, attr)
                row += 1

        footer = "[r] record/stop  [p] play reference  [<-/->] sentence  [?] help  [q] quit"
        if report is not None:
            footer = "[up/down] select word  [z] zoom in  " + footer
        if CLIPBOARD_MARKER in self.text_paths:
            footer += "  [enter] re-paste"
        self._addstr(rows - 1, 0, footer, cols, curses.A_DIM)
        self.stdscr.refresh()

    def _addstr(self, row, col, text, cols, attr=curses.A_NORMAL) -> None:
        try:
            self.stdscr.addstr(row, col, text[: max(cols - col, 0)], attr)
        except curses.error:
            pass

    def _draw_help(self) -> None:
        self.stdscr.erase()
        rows, cols = self.stdscr.getmaxyx()
        self._addstr(0, 0, "languify accent -- measurement reference", cols, curses.A_BOLD)
        row = 2
        for line in HELP_LINES:
            if row >= rows - FOOTER_ROWS:
                self._addstr(row - 1, MARGIN, "...", cols, curses.A_DIM)
                break
            self._addstr(row, MARGIN, line, cols, curses.A_NORMAL)
            row += 1
        self._addstr(rows - 1, 0, "[?] close", cols, curses.A_DIM)
        self.stdscr.refresh()

    def _draw_zoom(self) -> None:
        self.stdscr.erase()
        rows, cols = self.stdscr.getmaxyx()

        report = self.reports_by_sentence[self.current]
        wr = report[self.selected_word]
        anchored = _target_anchored(wr.phones)

        ref_report = self.reference_reports_by_sentence.get(self.current)
        ref_wr = ref_report[self.selected_word] if ref_report else None
        ref_anchored = _target_anchored(ref_wr.phones) if ref_wr else None

        word_visual = to_visual(wr.word, self.lang.needs_reshaping)
        header = (
            f"languify accent -- ZOOM  word {self.selected_word + 1}/{len(report)}: "
            f"{word_visual}   phone {self.zoom_phone + 1}/{max(len(anchored), 1)}"
        )
        self._addstr(0, 0, header, cols, curses.A_BOLD)

        if not anchored:
            self._addstr(2, MARGIN, "(no target phones for this word)", cols, curses.A_DIM)
            self._addstr(rows - 1, 0, "[z/esc] back  [q] quit", cols, curses.A_DIM)
            self.stdscr.refresh()
            return

        phone = anchored[self.zoom_phone]
        ref_phone = (
            ref_anchored[self.zoom_phone]
            if ref_anchored and self.zoom_phone < len(ref_anchored)
            else None
        )

        row = 2
        strip = "  ".join(
            (f"[{'ˈ' if i == wr.stressed_target_index else ''}{p.target or '-'}]"
             if i == self.zoom_phone else
             f" {'ˈ' if i == wr.stressed_target_index else ''}{p.target or '-'} ")
            for i, p in enumerate(anchored)
        )
        self._addstr(row, MARGIN, f"target:      {strip}", cols, curses.A_DIM)
        row += 2

        op_label = {Op.MATCH: "match", Op.SUBSTITUTE: "substituted", Op.DELETE: "missing"}[phone.op]
        you_attr = curses.A_NORMAL if phone.op == Op.MATCH else _score_color(1.0, self.has_color)
        self._addstr(
            row, MARGIN,
            f"you produced: {phone.recognized or '(nothing detected)'}   [{op_label}]",
            cols, you_attr,
        )
        row += 2

        if ref_wr is None:
            self._addstr(
                row, MARGIN,
                "(reference measurements unavailable -- " + (self.engine_error or "no engine configured") + ")",
                cols, curses.A_DIM,
            )
            row += 2
        elif ref_phone is None or ref_phone.recognized is None:
            self._addstr(row, MARGIN, "(reference didn't produce this phone either -- nothing to compare)", cols, curses.A_DIM)
            row += 2
        else:
            self._addstr(row, MARGIN, f"reference:    {ref_phone.recognized}", cols, curses.A_DIM)
            row += 2

        if phone.recognized is not None and ref_phone is not None and ref_phone.recognized is not None:
            cls = measure.phone_class(phone.target) if phone.target else "other"
            rows_to_show = [("duration", phone.duration_ms, ref_phone.duration_ms, "ms", "duration")]
            you_v, ref_v = phone.voicing, ref_phone.voicing
            rows_to_show.append((
                "voiced",
                you_v.voiced_fraction * 100 if you_v and you_v.voiced_fraction is not None else None,
                ref_v.voiced_fraction * 100 if ref_v and ref_v.voiced_fraction is not None else None,
                "%", "voicing",
            ))
            if cls == "vowel":
                you_f = phone.formants
                ref_f = ref_phone.formants
                rows_to_show.append(("F1", you_f.f1_hz if you_f else None, ref_f.f1_hz if ref_f else None, "Hz", "formant"))
                rows_to_show.append(("F2", you_f.f2_hz if you_f else None, ref_f.f2_hz if ref_f else None, "Hz", "formant"))
            elif cls == "stop":
                rows_to_show.append(("VOT", phone.vot_ms, ref_phone.vot_ms, "ms", "vot"))
            elif cls == "fricative":
                rows_to_show.append(("centroid", phone.centroid_hz, ref_phone.centroid_hz, "Hz", "formant"))

            if self.zoom_phone == wr.stressed_target_index:
                rows_to_show.append((
                    "stress",
                    wr.stress_ratio * 100 if wr.stress_ratio is not None else None,
                    ref_wr.stress_ratio * 100 if ref_wr.stress_ratio is not None else None,
                    "%", "duration",
                ))

            self._addstr(row, MARGIN, f"{'':13s}{'you':>10s}{'reference':>12s}{'delta':>10s}", cols, curses.A_DIM)
            row += 1
            for label, you, ref, unit, kind in rows_to_show:
                if row >= rows - FOOTER_ROWS:
                    break
                you_str = f"{you:.0f}{unit}" if you is not None else "--"
                ref_str = f"{ref:.0f}{unit}" if ref is not None else "--"
                if you is not None and ref is not None:
                    delta_str = f"{you - ref:+.0f}{unit}"
                    attr = _closeness_color(you, ref, kind, self.has_color)
                else:
                    delta_str = "--"
                    attr = curses.A_DIM
                line = f"{label:13s}{you_str:>10s}{ref_str:>12s}{delta_str:>10s}"
                self._addstr(row, MARGIN, line, cols, attr)
                row += 1

        self._addstr(rows - 1, 0, "[left/right] phone  [z/esc] back  [?] help  [q] quit", cols, curses.A_DIM)
        self.stdscr.refresh()

    # -- recording / scoring -------------------------------------------------

    def toggle_record(self) -> None:
        if self.status == "recording":
            self._stop_and_score()
        elif self.status in ("idle", "scored", "error") and self.sentences:
            self._stop_playback()  # don't record the reference audio into itself
            self.recorder.start()
            self.status = "recording"
            self.message = ""

    def toggle_playback(self) -> None:
        if self.playing:
            self._stop_playback()
            return
        if self.status == "recording" or not self.sentences:
            return
        if self.engine is None:
            self.message = self.engine_error or "no TTS engine configured for this language"
            return
        self.playing = self.player.play(self._reference_wav_for(self.current))

    def _stop_playback(self) -> None:
        if self.playing:
            self.player.stop()
            self.playing = False

    def _reference_wav_for(self, idx: int) -> str:
        pcm = self.reference_pcm_cache.get(idx)
        if pcm is None:
            pcm = self.engine.synthesize(self.sentences[idx])
            self.reference_pcm_cache[idx] = pcm
        path = os.path.join(self.cache_dir, f"ref_{idx}.wav")
        write_wav(path, pcm, self.engine.sample_rate)
        return path

    def tick(self) -> None:
        """Poll playback so `playing` (and thus the status line) clears
        itself once the reference audio finishes on its own, the same
        way languify.interactive's TTS reader notices a sentence ending."""
        if self.playing and not self.player.is_playing():
            self.playing = False

    def _stop_and_score(self) -> None:
        recording = self.recorder.stop()
        self.status = "scoring"
        self.message = ""
        self.draw()  # show "scoring..." before the blocking calls below --
        # this now also scores the reference audio (for the zoom view's
        # you-vs-native comparison) right after, so a first scoring of a
        # given sentence takes roughly twice as long as it will look from
        # this one status message.

        words = self.sentences[self.current].split()
        try:
            reports = score_sentence(
                recording.audio,
                recording.sample_rate,
                words,
                self.lang.name,
                nakdimon_path=self.nakdimon_path,
            )
            self.reports_by_sentence[self.current] = reports
            self.selected_word = 0
            self.status = "scored"
            self._ensure_reference_report(self.current)
        except (ValueError, RuntimeError) as exc:
            self.status = "error"
            self.message = str(exc)

        wav_path = os.path.join(self.cache_dir, f"{self.current}.wav")
        write_wav(wav_path, (recording.audio * 32767).astype("int16").tobytes(), recording.sample_rate)

        # Backstop for the stray-terminal-output problem above: clear()
        # (unlike erase()) forces curses to repaint every cell on the next
        # refresh() rather than diffing against what it thinks is already
        # there, so this overwrites any stray text that slipped through
        # despite the stdout/stderr redirection in main().
        self.stdscr.clear()

    def _ensure_reference_report(self, idx: int) -> None:
        """Score the reference (Piper/israwave) audio the same way the
        recording gets scored, so the zoom view has native measurements
        to diff your own against -- without this, zoom can only show your
        raw numbers with nothing to judge "closer" against. Failure here
        doesn't touch self.status/.message: the main score for `idx`
        already succeeded, and the zoom view just falls back to
        "reference unavailable" per word rather than losing the score
        you came here for."""
        if idx in self.reference_reports_by_sentence or self.engine is None:
            return
        try:
            pcm = self.reference_pcm_cache.get(idx)
            if pcm is None:
                pcm = self.engine.synthesize(self.sentences[idx])
                self.reference_pcm_cache[idx] = pcm
            audio = np.frombuffer(pcm, dtype=np.int16).astype(np.float32) / 32768.0
            words = self.sentences[idx].split()
            self.reference_reports_by_sentence[idx] = score_sentence(
                audio, self.engine.sample_rate, words, self.lang.name,
                nakdimon_path=self.nakdimon_path,
            )
        except (ValueError, RuntimeError):
            pass

    def move(self, delta: int) -> None:
        if self.status == "recording" or self.zoom:
            return  # don't let navigation interrupt a recording, or double as phone-stepping while zoomed
        self._stop_playback()
        new_idx = self.current + delta
        if 0 <= new_idx < len(self.sentences):
            self.current = new_idx
            self.status = "scored" if self.current in self.reports_by_sentence else "idle"
            self.message = ""
            self.selected_word = 0
            self.layout()

    # -- word selection / zoom -----------------------------------------------

    def select_word(self, delta: int) -> None:
        if self.zoom or self.status in ("recording", "scoring"):
            return
        report = self.reports_by_sentence.get(self.current)
        if not report:
            return
        self.selected_word = max(0, min(len(report) - 1, self.selected_word + delta))

    def enter_zoom(self) -> None:
        if self.status in ("recording", "scoring"):
            return
        report = self.reports_by_sentence.get(self.current)
        if not report:
            return
        self.zoom = True
        self.zoom_phone = 0

    def exit_zoom(self) -> None:
        self.zoom = False

    def zoom_move(self, delta: int) -> None:
        if not self.zoom:
            return
        report = self.reports_by_sentence[self.current]
        anchored = _target_anchored(report[self.selected_word].phones)
        if not anchored:
            return
        self.zoom_phone = max(0, min(len(anchored) - 1, self.zoom_phone + delta))

    def close(self) -> None:
        if self.recorder.active:
            self.recorder.stop()
        self.player.stop()
        shutil.rmtree(self.cache_dir, ignore_errors=True)


def _run(stdscr, text_paths: List[str], lang: LanguageConfig, nakdimon_path: Optional[str]) -> None:
    curses.curs_set(0)
    stdscr.timeout(POLL_MS)

    session = Session(stdscr, text_paths, lang, nakdimon_path)
    try:
        session.draw()
        while True:
            session.tick()
            ch = stdscr.getch()
            if ch == -1:
                session.draw()
                continue
            elif ch == curses.KEY_RESIZE:
                session.layout()
            elif ch in (ord("q"), ord("Q")):
                break
            elif ch == ord("?"):
                session.help = not session.help
            elif session.help:
                pass  # swallow every other key while the help overlay is open
            elif ch in (ord("r"), ord("R")):
                session.toggle_record()
            elif ch in (ord("p"), ord("P")):
                session.toggle_playback()
            elif ch in (ord("z"), ord("Z"), 27):  # 27 = Esc
                session.exit_zoom() if session.zoom else session.enter_zoom()
            elif ch == curses.KEY_LEFT:
                session.zoom_move(-1) if session.zoom else session.move(-1)
            elif ch == curses.KEY_RIGHT:
                session.zoom_move(1) if session.zoom else session.move(1)
            elif ch == curses.KEY_UP:
                session.select_word(-1)
            elif ch == curses.KEY_DOWN:
                session.select_word(1)
            elif ch in (curses.KEY_ENTER, 10, 13):
                session.refresh_clipboard()
            session.draw()
    finally:
        session.close()


def main(text_paths: List[str], lang: LanguageConfig, nakdimon_path: Optional[str] = None) -> None:
    locale.setlocale(locale.LC_ALL, "")  # must precede initscr for unicode addstr

    # The first scoring call lazily imports torch/transformers/torchaudio,
    # which print straight to the terminal (tqdm progress bars for model
    # loading/downloads, huggingface_hub's "set HF_TOKEN" notice) -- those
    # writes land outside curses' own screen model, so a later erase()+
    # refresh() doesn't necessarily clean them up (curses only repaints
    # cells it thinks changed since its last refresh). Fix: redirect the
    # OS-level stderr fd to a log file for the session (covers native-code
    # writes too, not just Python's); stdout is left alone at the fd level
    # since curses itself renders through fd 1, and touching that risks
    # breaking the UI rather than just a cosmetic glitch -- Python-level
    # `sys.stdout` is swapped instead, which catches stray print() calls
    # without affecting curses' own lower-level writes to that same fd.
    log_path = os.path.join(tempfile.gettempdir(), "languify-accent.log")
    log_file = open(log_path, "a", buffering=1)
    saved_stderr_fd = os.dup(2)
    saved_stdout = sys.stdout
    os.dup2(log_file.fileno(), 2)
    sys.stdout = log_file
    try:
        curses.wrapper(_run, text_paths, lang, nakdimon_path)
    finally:
        sys.stdout = saved_stdout
        os.dup2(saved_stderr_fd, 2)
        os.close(saved_stderr_fd)
        log_file.close()
