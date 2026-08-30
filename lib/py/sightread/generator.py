"""Randomly generate sight-reading exercises (melodies and triad studies)
that get progressively harder with `level`.

Each level widens three things at once: how many key-signature accidentals
are in play, the pitch range / leap size, and the rhythmic vocabulary.
"""

import random

from sightread import theory

NUM_LEVELS = 8


def _level_spec(level):
    level = max(1, min(NUM_LEVELS, level))
    return LEVELS[level - 1]


LEVELS = [
    # 1: dead simple -- C major/A minor, stepwise quarter notes only.
    dict(max_accidentals=0, exercise_types={'melody': 1.0}, range=(60, 74),
         base_durations=[1.0], subdivisions=[], rest_prob=0.0, max_step=1,
         triad_inversions=[0], chords_per_measure=1, time_sigs=[(4, 4)]),
    # 2: add half notes and small leaps, still no accidentals.
    dict(max_accidentals=0, exercise_types={'melody': 1.0}, range=(59, 76),
         base_durations=[1.0, 1.0, 2.0], subdivisions=[], rest_prob=0.05, max_step=2,
         triad_inversions=[0], chords_per_measure=1, time_sigs=[(4, 4)]),
    # 3: introduce root-position triad studies; one key signature accidental.
    dict(max_accidentals=1, exercise_types={'melody': 0.6, 'triads': 0.4}, range=(57, 77),
         base_durations=[1.0, 1.0, 2.0, 4.0], subdivisions=[], rest_prob=0.08, max_step=2,
         triad_inversions=[0], chords_per_measure=1, time_sigs=[(4, 4)]),
    # 4: eighth-note pairs, wider range, up to 2 accidentals.
    dict(max_accidentals=2, exercise_types={'melody': 0.55, 'triads': 0.45}, range=(55, 79),
         base_durations=[1.0, 1.0, 2.0], subdivisions=[(0.4, [0.5, 0.5])], rest_prob=0.1,
         max_step=3, triad_inversions=[0, 1], chords_per_measure=2, time_sigs=[(4, 4), (3, 4)]),
    # 5: first/second inversions, up to 3 accidentals.
    dict(max_accidentals=3, exercise_types={'melody': 0.5, 'triads': 0.5}, range=(53, 81),
         base_durations=[1.0, 1.0, 2.0], subdivisions=[(0.45, [0.5, 0.5])], rest_prob=0.12,
         max_step=4, triad_inversions=[0, 1, 2], chords_per_measure=2, time_sigs=[(4, 4), (3, 4)]),
    # 6: dotted rhythms, wider range with more ledger lines, up to 4 accidentals.
    dict(max_accidentals=4, exercise_types={'melody': 0.5, 'triads': 0.5}, range=(52, 84),
         base_durations=[1.0, 2.0], subdivisions=[(0.35, [0.5, 0.5]), (0.2, [1.5, 0.5])],
         rest_prob=0.14, max_step=5, triad_inversions=[0, 1, 2], chords_per_measure=4,
         time_sigs=[(4, 4), (3, 4)]),
    # 7: sixteenths, up to 5 accidentals, quarter-note chord changes.
    dict(max_accidentals=5, exercise_types={'melody': 0.45, 'triads': 0.55}, range=(52, 86),
         base_durations=[1.0, 2.0], subdivisions=[(0.3, [0.5, 0.5]), (0.25, [0.25] * 4),
                                                   (0.15, [0.75, 0.25])],
         rest_prob=0.16, max_step=6, triad_inversions=[0, 1, 2], chords_per_measure=4,
         time_sigs=[(4, 4), (3, 4)]),
    # 8: full range, up to 6 accidentals, everything mixed.
    dict(max_accidentals=6, exercise_types={'melody': 0.4, 'triads': 0.6}, range=(52, 88),
         base_durations=[1.0, 2.0], subdivisions=[(0.3, [0.5, 0.5]), (0.25, [0.25] * 4),
                                                   (0.2, [0.75, 0.25])],
         rest_prob=0.18, max_step=7, triad_inversions=[0, 1, 2], chords_per_measure=4,
         time_sigs=[(4, 4), (3, 4)]),
]

SYSTEMS_PER_SAMPLE = 2
MEASURES_PER_SYSTEM = 4


def _weighted_choice(weights_dict, rng):
    items = list(weights_dict.items())
    total = sum(w for _, w in items)
    r = rng.uniform(0, total)
    upto = 0
    for value, w in items:
        upto += w
        if r <= upto:
            return value
    return items[-1][0]


def _fill_measure_slots(beats_total, spec, rng):
    """Coarse duration slots (quarters/halves/wholes/rests) summing exactly
    to beats_total, then randomly subdivide some quarter slots."""
    pool = [d for d in spec['base_durations'] if d <= beats_total]
    slots = []
    remaining = beats_total
    while remaining > 1e-9:
        choices = [d for d in pool if d <= remaining + 1e-9]
        if not choices:
            choices = [min(pool)]
        d = rng.choice(choices)
        is_rest = rng.random() < spec['rest_prob']
        slots.append((d, is_rest))
        remaining -= d

    expanded = []
    for d, is_rest in slots:
        if not is_rest and abs(d - 1.0) < 1e-9 and spec['subdivisions'] and rng.random() < 0.5:
            r = rng.random()
            upto = 0
            chosen = None
            for prob, parts in spec['subdivisions']:
                upto += prob
                if r <= upto:
                    chosen = parts
                    break
            if chosen:
                for part in chosen:
                    expanded.append((part, False))
                continue
        expanded.append((d, is_rest))
    return expanded


def _generate_melody(key, spec, time_sig, rng):
    low, high = spec['range']
    scale_run = theory.build_scale_run(key, low, high)
    idx = len(scale_run) // 2
    beats_per_measure = time_sig[0] * (4 / time_sig[1])

    systems = []
    for _ in range(SYSTEMS_PER_SAMPLE):
        measures = []
        for _ in range(MEASURES_PER_SYSTEM):
            events = []
            for beats, is_rest in _fill_measure_slots(beats_per_measure, spec, rng):
                if is_rest:
                    events.append({'notes': [], 'beats': beats})
                    continue
                step = rng.randint(-spec['max_step'], spec['max_step'])
                if step == 0:
                    step = rng.choice([-1, 1])
                idx = max(0, min(len(scale_run) - 1, idx + step))
                events.append({'notes': [scale_run[idx]], 'beats': beats})
            measures.append(events)
        systems.append(measures)
    return systems


def _generate_triads(key, spec, time_sig, rng):
    low, high = spec['range']
    beats_per_measure = time_sig[0] * (4 / time_sig[1])
    per_measure = min(spec['chords_per_measure'], int(beats_per_measure))
    chord_beats = beats_per_measure / per_measure

    prev_degree = None
    systems = []
    for _ in range(SYSTEMS_PER_SAMPLE):
        measures = []
        for _ in range(MEASURES_PER_SYSTEM):
            events = []
            for _ in range(per_measure):
                degree = rng.randint(0, 6)
                if degree == prev_degree:
                    degree = (degree + rng.choice([1, 2, 3])) % 7
                prev_degree = degree
                inversion = rng.choice(spec['triad_inversions'])
                notes = theory.build_triad(key, degree, inversion)
                notes = theory.transpose_to_range(notes, low, high)
                events.append({'notes': notes, 'beats': chord_beats})
            measures.append(events)
        systems.append(measures)
    return systems


def generate_sample(level, rng=None):
    """Returns (key, time_sig, systems, exercise_type) for one practice sample."""
    rng = rng or random
    spec = _level_spec(level)
    key = theory.random_key(spec['max_accidentals'], rng)
    time_sig = rng.choice(spec['time_sigs'])
    exercise_type = _weighted_choice(spec['exercise_types'], rng)

    if exercise_type == 'triads':
        systems = _generate_triads(key, spec, time_sig, rng)
    else:
        systems = _generate_melody(key, spec, time_sig, rng)

    return key, time_sig, systems, exercise_type
