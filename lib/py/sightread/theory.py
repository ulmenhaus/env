"""Music theory primitives: keys, scales, triads, staff positions, MIDI numbers.

A "note" is represented throughout as a (letter, accidental, octave) tuple:
  letter:     one of 'C' 'D' 'E' 'F' 'G' 'A' 'B'
  accidental: -1 (flat), 0 (natural), +1 (sharp)
  octave:     scientific pitch notation octave, e.g. middle C is ('C', 0, 4)
"""

import random
from dataclasses import dataclass, field

LETTERS = ['C', 'D', 'E', 'F', 'G', 'A', 'B']
LETTER_SEMITONE = {'C': 0, 'D': 2, 'E': 4, 'F': 5, 'G': 7, 'A': 9, 'B': 11}
LETTER_INDEX = {letter: i for i, letter in enumerate(LETTERS)}

SHARP_ORDER = ['F', 'C', 'G', 'D', 'A', 'E', 'B']
FLAT_ORDER = ['B', 'E', 'A', 'D', 'G', 'C', 'F']

MAJOR_TONICS_SHARPS = ['C', 'G', 'D', 'A', 'E', 'B', 'F', 'C']
MAJOR_TONICS_SHARPS_ACC = [0, 0, 0, 0, 0, 0, 1, 1]  # tonic itself is sharp for F#, C#
MAJOR_TONICS_FLATS = ['C', 'F', 'B', 'E', 'A', 'D', 'G', 'C']
MAJOR_TONICS_FLATS_ACC = [0, 0, -1, -1, -1, -1, -1, -1]

# Reference for staff-step math: E4 sits on the bottom line of the treble
# staff. Step 0 == top line, step 4 == bottom line, half-steps are the
# spaces in between; values grow downward on the page as pitch falls.
STAFF_REF_ABS_INDEX = 4 * 7 + LETTER_INDEX['E']
STAFF_REF_STEP = 4.0


@dataclass(frozen=True)
class Key:
    tonic_letter: str
    tonic_accidental: int
    mode: str  # 'major' or 'minor'
    num_accidentals: int  # signed: positive = sharps, negative = flats
    accidentals: dict = field(default_factory=dict)  # letter -> -1/0/+1

    @property
    def name(self):
        acc = {1: '#', -1: 'b'}.get(self.tonic_accidental, '')
        return f"{self.tonic_letter}{acc} {self.mode}"


def _major_key_for_accidentals(n):
    """n >= 0 => n sharps, n < 0 => -n flats."""
    if n >= 0:
        tonic = MAJOR_TONICS_SHARPS[n]
        tonic_acc = MAJOR_TONICS_SHARPS_ACC[n]
        accidentals = {letter: 1 for letter in SHARP_ORDER[:n]}
    else:
        n = -n
        tonic = MAJOR_TONICS_FLATS[n]
        tonic_acc = MAJOR_TONICS_FLATS_ACC[n]
        accidentals = {letter: -1 for letter in FLAT_ORDER[:n]}
    return tonic, tonic_acc, accidentals


def major_key(n):
    tonic, tonic_acc, accidentals = _major_key_for_accidentals(n)
    return Key(tonic, tonic_acc, 'major', n, accidentals)


def relative_minor_key(major):
    """The natural-minor key sharing major's key signature."""
    scale = scale_letters(major)
    minor_letter, minor_acc = scale[5]  # submediant == relative minor tonic
    return Key(minor_letter, minor_acc, 'minor', major.num_accidentals, major.accidentals)


def scale_letters(key):
    """Seven (letter, accidental, diatonic_index) tuples starting on the tonic,
    diatonic_index counting letter-steps from C (used for octave bookkeeping)."""
    start = LETTER_INDEX[key.tonic_letter]
    out = []
    for i in range(7):
        letter = LETTERS[(start + i) % 7]
        acc = key.accidentals.get(letter, 0)
        out.append((letter, acc))
    return out


def pitch_class(letter, accidental):
    return (LETTER_SEMITONE[letter] + accidental) % 12


def midi_number(letter, accidental, octave):
    return (octave + 1) * 12 + LETTER_SEMITONE[letter] + accidental


def staff_step(letter, octave):
    """Vertical position in half-line-steps; 0 == top line, 4 == bottom line."""
    abs_index = octave * 7 + LETTER_INDEX[letter]
    return STAFF_REF_STEP - 0.5 * (abs_index - STAFF_REF_ABS_INDEX)


def build_scale_run(key, low_midi, high_midi):
    """All (letter, accidental, octave) triples in `key` whose MIDI number
    falls within [low_midi, high_midi], ascending."""
    letters = scale_letters(key)
    notes = []
    for octave in range(-1, 10):
        for letter, acc in letters:
            m = midi_number(letter, acc, octave)
            if low_midi <= m <= high_midi:
                notes.append((letter, acc, octave))
    notes.sort(key=lambda n: midi_number(*n))
    return notes


def build_triad(key, root_index, inversion=0):
    """Diatonic triad stacked in thirds on scale degree `root_index` (index
    into one octave of `scale_letters(key)`), returned low-to-high with the
    requested inversion applied (0=root, 1=first, 2=second)."""
    letters = scale_letters(key)
    root_letter, root_acc = letters[root_index % 7]
    # anchor octave arbitrarily at 4; caller re-octaves via nearest-fit.
    base_octave = 4
    root_abs = base_octave * 7 + LETTER_INDEX[root_letter]

    members = []
    for step in (0, 2, 4):
        idx = (root_index + step) % 7
        letter, acc = letters[idx]
        abs_idx = base_octave * 7 + LETTER_INDEX[letter]
        # push into the correct octave relative to the root
        while abs_idx < root_abs:
            abs_idx += 7
        octave = abs_idx // 7
        members.append((letter, acc, octave))

    for _ in range(inversion % 3):
        low = members.pop(0)
        members.append((low[0], low[1], low[2] + 1))

    return members


def transpose_to_range(notes, low_midi, high_midi):
    """Shift a group of notes by whole octaves so their midpoint lands
    inside [low_midi, high_midi] as best as possible."""
    mids = [midi_number(*n) for n in notes]
    target_mid = (low_midi + high_midi) / 2
    current_mid = sum(mids) / len(mids)
    shift_octaves = round((target_mid - current_mid) / 12)
    return [(l, a, o + shift_octaves) for (l, a, o) in notes]


def random_key(max_accidentals, rng=None):
    rng = rng or random
    n = rng.randint(-max_accidentals, max_accidentals)
    major = major_key(n)
    if rng.random() < 0.35:
        return relative_minor_key(major)
    return major
