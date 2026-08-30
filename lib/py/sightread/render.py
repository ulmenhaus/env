"""Render a sight-reading exercise to a PNG: white ink on a black page.

No music font is used anywhere -- the clef, accidentals, noteheads, stems,
flags and beams are all drawn as vector primitives with Pillow so the tool
has no dependency on any particular system font.
"""

import os

from PIL import Image, ImageDraw, ImageFont

from sightread import theory

WHITE = (255, 255, 255)
BLACK = (0, 0, 0)

_FONT_CANDIDATES = [
    "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
    "/usr/local/lib/python3.12/dist-packages/matplotlib/mpl-data/fonts/ttf/DejaVuSans-Bold.ttf",
]


def _load_font(size):
    for path in _FONT_CANDIDATES:
        if os.path.exists(path):
            return ImageFont.truetype(path, size)
    return ImageFont.load_default()


class Layout:
    def __init__(self, line_gap=22):
        self.line_gap = line_gap
        self.staff_height = line_gap * 4
        self.left_margin = int(line_gap * 3.2)
        self.right_margin = int(line_gap * 1.2)
        self.system_gap = int(line_gap * 7.5)
        self.top_margin = int(line_gap * 4.5)
        self.bottom_margin = int(line_gap * 3.0)
        self.stem_len = line_gap * 3.4
        self.notehead_rx = line_gap * 0.52
        self.notehead_ry = line_gap * 0.38
        self.line_width = max(2, line_gap // 12)
        self.stem_width = max(2, line_gap // 14)


def y_for_step(step, staff_top_y, lg):
    return staff_top_y + step * lg


# ---------------------------------------------------------------- clef ----

def draw_treble_clef(draw, x, staff_top_y, lg, color=WHITE):
    """A hand-built approximation of a G clef: a big lower loop, a small
    "eye" loop wrapping the G line (step 3), and a curvy spine connecting
    them and rising above the staff with a small curl at the top.

    Coordinates are given as (x_offset_in_lg, step) pairs -- step 0 is the
    top line, step 4 the bottom line, matching theory.staff_step -- then
    scaled into pixels.
    """
    def px(ox, step):
        return (x + ox * lg, staff_top_y + step * lg)

    width = max(3, int(lg * 0.20))

    spine = [
        (0.15, 6.7),
        (-0.35, 6.05),
        (-0.35, 5.25),
        (0.2, 4.6),
        (0.75, 3.9),
        (0.85, 3.0),
        (0.45, 2.15),
        (-0.05, 1.5),
        (-0.25, 0.75),
        (0.0, 0.05),
        (0.55, -0.6),
        (0.95, -1.35),
        (0.75, -2.0),
        (0.2, -2.1),
        (-0.1, -1.7),
        (0.05, -1.3),
    ]
    px_pts = [px(ox, step) for ox, step in spine]
    draw.line(px_pts, fill=color, width=width, joint="curve")
    r = width / 2
    for p in (px_pts[0], px_pts[-1]):
        draw.ellipse([p[0] - r, p[1] - r, p[0] + r, p[1] + r], fill=color)

    # big lower loop
    lo = px(0.05, 5.65)
    lw_bbox = [lo[0] - 0.78 * lg, lo[1] - 0.95 * lg, lo[0] + 0.78 * lg, lo[1] + 0.95 * lg]
    draw.ellipse(lw_bbox, outline=color, width=width)

    # small eye loop wrapping the G line
    eye = px(0.45, 3.0)
    eye_bbox = [eye[0] - 0.42 * lg, eye[1] - 0.55 * lg, eye[0] + 0.42 * lg, eye[1] + 0.55 * lg]
    draw.ellipse(eye_bbox, outline=color, width=max(2, int(lg * 0.16)))

    return x + 1.55 * lg  # advance width


# ---------------------------------------------------------- accidentals ----

def draw_sharp(draw, x, y, lg, color=WHITE):
    """y = pixel position of the pitch this sharp applies to."""
    hx = lg * 0.22
    half_h = lg * 0.85
    w = max(2, int(lg * 0.13))
    draw.line([(x - hx, y - half_h), (x - hx, y + half_h)], fill=color, width=w)
    draw.line([(x + hx, y - half_h), (x + hx, y + half_h)], fill=color, width=w)
    bar_w = max(3, int(lg * 0.22))
    tilt = lg * 0.13
    bar_half = hx + lg * 0.12
    for bar_y in (y - lg * 0.30, y + lg * 0.30):
        draw.line(
            [(x - bar_half, bar_y + tilt), (x + bar_half, bar_y - tilt)],
            fill=color, width=bar_w,
        )
    return x + lg * 0.85


def draw_flat(draw, x, y, lg, color=WHITE):
    """y = pixel position of the pitch this flat applies to."""
    stem_top = y - lg * 1.3
    stem_bottom = y + lg * 0.25
    w = max(2, int(lg * 0.16))
    draw.line([(x, stem_top), (x, stem_bottom)], fill=color, width=w)
    bbox = [x - lg * 0.05, y - lg * 0.55, x + lg * 0.62, y + lg * 0.25]
    draw.arc(bbox, start=-100, end=110, fill=color, width=max(3, int(lg * 0.22)))
    return x + lg * 0.85


def draw_natural(draw, x, y, lg, color=WHITE):
    h = lg * 1.7
    hx = lg * 0.22
    w = max(2, int(lg * 0.16))
    draw.line([(x - hx, y - h / 2), (x - hx, y + h / 2 - lg * 0.35)], fill=color, width=w)
    draw.line([(x + hx, y - h / 2 + lg * 0.35), (x + hx, y + h / 2)], fill=color, width=w)
    bar_w = max(2, int(lg * 0.20))
    draw.line([(x - hx, y - h / 2 + lg * 0.35), (x + hx, y - h / 2 + lg * 0.35 + lg * 0.02)],
              fill=color, width=bar_w)
    draw.line([(x - hx, y + h / 2 - lg * 0.35 - lg * 0.02), (x + hx, y + h / 2 - lg * 0.35)],
              fill=color, width=bar_w)
    return x + lg * 0.7


ACCIDENTAL_DRAWERS = {1: draw_sharp, -1: draw_flat, 0: draw_natural}

# Canonical treble-clef (letter, octave) placement for each accidental in
# the standard key-signature order -- used both for the key signature glyphs
# and derived the same way real engravings place them.
_SHARP_PLACEMENT = {'F': 5, 'C': 5, 'G': 5, 'D': 5, 'A': 5, 'E': 5, 'B': 5}
_FLAT_PLACEMENT = {'B': 4, 'E': 5, 'A': 4, 'D': 5, 'G': 4, 'C': 5, 'F': 4}


def draw_key_signature(draw, x, staff_top_y, lg, key, color=WHITE):
    if key.num_accidentals == 0:
        return x
    if key.num_accidentals > 0:
        order = theory.SHARP_ORDER[: key.num_accidentals]
        placement = _SHARP_PLACEMENT
        drawer = draw_sharp
    else:
        order = theory.FLAT_ORDER[: -key.num_accidentals]
        placement = _FLAT_PLACEMENT
        drawer = draw_flat
    for letter in order:
        step = theory.staff_step(letter, placement[letter])
        y = y_for_step(step, staff_top_y, lg)
        x = drawer(draw, x, y, lg, color)
    return x + lg * 0.55


# --------------------------------------------------------- time signature --

def draw_time_signature(draw, x, staff_top_y, lg, beats_per_measure, beat_unit, color=WHITE):
    font = _load_font(int(lg * 1.8))
    top = str(beats_per_measure)
    bot = str(beat_unit)
    for text, step_center in ((top, 1.0), (bot, 3.0)):
        bbox = draw.textbbox((0, 0), text, font=font)
        tw, th = bbox[2] - bbox[0], bbox[3] - bbox[1]
        ty = y_for_step(step_center, staff_top_y, lg) - th / 2 - bbox[1]
        draw.text((x, ty), text, font=font, fill=color)
    return x + lg * 1.3


# -------------------------------------------------------------- noteheads --

DURATION_STYLES = {
    # beats -> (open_head, has_stem, num_flags, dotted)
    4.0: (True, False, 0, False),
    3.0: (True, True, 0, True),
    2.0: (True, True, 0, False),
    1.5: (False, True, 0, True),
    1.0: (False, True, 0, False),
    0.75: (False, True, 1, True),
    0.5: (False, True, 1, False),
    0.25: (False, True, 2, False),
}


def duration_style(beats):
    return DURATION_STYLES.get(round(beats, 4), (False, True, 0, False))


def draw_ledger_lines(draw, x, steps, lg, staff_top_y, color=WHITE):
    half_w = lg * 0.85
    for step in steps:
        if step < -0.01:
            n = int((-step) // 1)
            for i in range(1, n + 1):
                y = y_for_step(-i, staff_top_y, lg)
                draw.line([(x - half_w, y), (x + half_w, y)], fill=color, width=max(2, int(lg * 0.11)))
        elif step > 4.01:
            n = int((step - 4) // 1)
            for i in range(1, n + 1):
                y = y_for_step(4 + i, staff_top_y, lg)
                draw.line([(x - half_w, y), (x + half_w, y)], fill=color, width=max(2, int(lg * 0.11)))


def draw_chord(draw, x, note_steps, beats, lg, staff_top_y, layout, color=WHITE):
    """note_steps: list of staff-step floats (already resolved from pitches)."""
    open_head, has_stem, num_flags, dotted = duration_style(beats)
    rx, ry = layout.notehead_rx, layout.notehead_ry

    draw_ledger_lines(draw, x, note_steps, lg, staff_top_y, color)

    ys = [y_for_step(s, staff_top_y, lg) for s in note_steps]
    for y in ys:
        bbox = [x - rx, y - ry, x + rx, y + ry]
        if open_head:
            draw.ellipse(bbox, outline=color, width=max(2, int(lg * 0.13)))
        else:
            draw.ellipse(bbox, fill=color)
        if dotted:
            draw.ellipse([x + rx * 1.7, y - lg * 0.06, x + rx * 1.7 + lg * 0.16, y + lg * 0.10],
                         fill=color)

    if not has_stem:
        return

    middle_step = 2.0
    avg_step = sum(note_steps) / len(note_steps)
    stem_up = avg_step >= middle_step

    if stem_up:
        note_y = max(ys)  # lowest note (largest step) anchors an up-stem
        top_y = min(ys)
        stem_x = x + rx - layout.stem_width / 2
        stem_top = top_y - layout.stem_len
        draw.line([(stem_x, note_y), (stem_x, stem_top)], fill=color, width=layout.stem_width)
        flag_origin = (stem_x, stem_top)
    else:
        note_y = min(ys)  # highest note (smallest step) anchors a down-stem
        bottom_y = max(ys)
        stem_x = x - rx + layout.stem_width / 2
        stem_bottom = bottom_y + layout.stem_len
        draw.line([(stem_x, note_y), (stem_x, stem_bottom)], fill=color, width=layout.stem_width)
        flag_origin = (stem_x, stem_bottom)

    draw_flags(draw, flag_origin, num_flags, stem_up, lg, color)


def draw_flags(draw, tip, num_flags, stem_up, lg, color=WHITE):
    """Flag(s) hanging off a stem tip: a curved teardrop, stacked toward the
    notehead for sixteenths and shorter."""
    tip_x, tip_y = tip
    step_toward_head = 0.42 * lg
    shape = [(0.0, 0.0), (0.12, 0.08), (0.72, 0.30), (0.55, 0.62), (0.24, 0.52), (0.04, 0.22)]
    sign = 1 if stem_up else -1
    for i in range(num_flags):
        oy = tip_y + sign * i * step_toward_head
        pts = [(tip_x + bx * lg, oy + sign * by * lg) for bx, by in shape]
        draw.polygon(pts, fill=color)


def draw_rest(draw, x, beats, lg, staff_top_y, color=WHITE):
    mid_y = y_for_step(2.0, staff_top_y, lg)
    if round(beats, 4) == 4.0:
        y = y_for_step(1.0, staff_top_y, lg)
        draw.rectangle([x - lg * 0.35, y, x + lg * 0.35, y + lg * 0.25], fill=color)
    elif round(beats, 4) == 2.0:
        y = y_for_step(2.0, staff_top_y, lg)
        draw.rectangle([x - lg * 0.35, y - lg * 0.25, x + lg * 0.35, y], fill=color)
    elif round(beats, 4) in (1.0, 1.5):
        # simplified quarter-rest glyph: a zig-zag stroke
        pts = [
            (x - lg * 0.05, mid_y - lg * 0.9),
            (x + lg * 0.25, mid_y - lg * 0.45),
            (x - lg * 0.15, mid_y - lg * 0.05),
            (x + lg * 0.2, mid_y + lg * 0.35),
            (x - lg * 0.1, mid_y + lg * 0.95),
        ]
        draw.line(pts, fill=color, width=max(3, int(lg * 0.16)), joint="curve")
        if round(beats, 4) == 1.5:
            draw.ellipse([x + lg * 0.35, mid_y - lg * 0.1, x + lg * 0.5, mid_y + lg * 0.05], fill=color)
    else:
        y = mid_y
        draw.ellipse([x - lg * 0.1, y - lg * 0.1, x + lg * 0.1, y + lg * 0.1], fill=color)
        draw.line([(x - lg * 0.15, y - lg * 0.15), (x + lg * 0.35, y + lg * 0.4)], fill=color,
                  width=max(2, int(lg * 0.12)))


def draw_beam_group(draw, x_positions, note_steps_list, beats, lg, staff_top_y, layout, color=WHITE):
    """Draw a set of >=2 eighth/sixteenth notes joined by beam(s) instead of
    individual flags. note_steps_list[i] is the list of staff-steps for
    chord i (usually a single-note melody, so length 1)."""
    _, _, num_flags, _ = duration_style(beats)
    avg_all = sum(s for steps in note_steps_list for s in steps) / sum(len(s) for s in note_steps_list)
    stem_up = avg_all >= 2.0
    rx = layout.notehead_rx

    # The beam is kept flat (rather than sloped to the melodic contour) so
    # that wide leaps between beamed notes never produce a near-vertical
    # beam; each note's stem simply stretches to reach it.
    if stem_up:
        beam_y = min(y_for_step(min(steps), staff_top_y, lg) for steps in note_steps_list) - layout.stem_len
    else:
        beam_y = max(y_for_step(max(steps), staff_top_y, lg) for steps in note_steps_list) + layout.stem_len

    stem_xs = []
    for x, steps in zip(x_positions, note_steps_list):
        draw_ledger_lines(draw, x, steps, lg, staff_top_y, color)
        ry = layout.notehead_ry
        for s in steps:
            y = y_for_step(s, staff_top_y, lg)
            draw.ellipse([x - rx, y - ry, x + rx, y + ry], fill=color)
        if stem_up:
            note_y = y_for_step(max(steps), staff_top_y, lg)
            stem_x = x + rx - layout.stem_width / 2
        else:
            note_y = y_for_step(min(steps), staff_top_y, lg)
            stem_x = x - rx + layout.stem_width / 2
        draw.line([(stem_x, note_y), (stem_x, beam_y)], fill=color, width=layout.stem_width)
        stem_xs.append(stem_x)
    stem_ends = [beam_y] * len(stem_xs)

    beam_w = max(3, int(lg * 0.28))
    for level in range(num_flags):
        offset = level * beam_w * 1.7 * (1 if stem_up else -1)
        draw.line(
            [(stem_xs[0], stem_ends[0] + offset), (stem_xs[-1], stem_ends[-1] + offset)],
            fill=color, width=beam_w,
        )


# ------------------------------------------------------------ main render --

def render_exercise(systems, key, beats_per_measure, beat_unit, out_path,
                     width=1400, layout=None, title=None):
    """systems: list of systems; each system is a list of measures; each
    measure is a list of event dicts:
        {'notes': [(letter,acc,octave), ...], 'beats': float}
    an empty 'notes' list means a rest.
    """
    layout = layout or Layout()
    lg = layout.line_gap

    # The clef itself reaches a bit above the top line and further below the
    # bottom line; widen margins/system-gap further still if any note in the
    # exercise needs more room than that (e.g. wide-range high levels), so
    # ledger lines and noteheads never run off the edge of the page.
    all_steps = [theory.staff_step(n[0], n[2]) for measures in systems for events in measures
                 for e in events for n in e['notes']]
    extra_above = max([0.0] + [-s for s in all_steps if s < 0])
    extra_below = max([0.0] + [s - 4 for s in all_steps if s > 4])
    above_steps = max(2.15, extra_above) + 1.0
    below_steps = max(2.7, extra_below) + 1.0

    top_margin = max(layout.top_margin, int(above_steps * lg))
    bottom_margin = max(layout.bottom_margin, int(below_steps * lg))
    system_gap = max(layout.system_gap, int((above_steps + below_steps) * lg))

    n_systems = len(systems)
    height = top_margin + bottom_margin + n_systems * layout.staff_height + \
        (n_systems - 1) * system_gap
    if title:
        height += int(lg * 2.2)

    img = Image.new("RGB", (width, height), BLACK)
    draw = ImageDraw.Draw(img)

    title_offset = 0
    if title:
        font = _load_font(int(lg * 1.1))
        draw.text((layout.left_margin * 0.3, lg * 0.6), title, font=font, fill=WHITE)
        title_offset = int(lg * 2.2)

    staff_top = top_margin + title_offset
    content_right = width - layout.right_margin

    for sys_idx, measures in enumerate(systems):
        y0 = staff_top + sys_idx * (layout.staff_height + system_gap)
        for i in range(5):
            y = y_for_step(i, y0, lg)
            draw.line([(layout.left_margin, y), (content_right, y)], fill=WHITE, width=layout.line_width)

        x = layout.left_margin
        x = draw_treble_clef(draw, x + lg * 0.3, y0, lg)
        x = draw_key_signature(draw, x, y0, lg, key)
        if sys_idx == 0:
            x = draw_time_signature(draw, x, y0, lg, beats_per_measure, beat_unit)

        n_measures = len(measures)
        measure_width = (content_right - x) / max(1, n_measures)

        for m_idx, events in enumerate(measures):
            m_left = x + m_idx * measure_width
            m_right = m_left + measure_width
            draw.line([(m_left, y_for_step(0, y0, lg)), (m_left, y_for_step(4, y0, lg))],
                      fill=WHITE, width=layout.line_width)

            total_beats = sum(e['beats'] for e in events) or 1
            inner_left = m_left + measure_width * 0.12
            inner_right = m_right - measure_width * 0.06
            avail = inner_right - inner_left

            cursor = 0.0
            positions = []
            for e in events:
                frac = cursor / total_beats
                positions.append(inner_left + frac * avail)
                cursor += e['beats']

            # proportional spacing alone can crowd fast subdivisions (e.g.
            # four sixteenths in one beat) into overlapping noteheads --
            # enforce a floor on the gap between consecutive notes.
            min_gap = layout.notehead_rx * 2.5
            for i in range(1, len(positions)):
                if positions[i] - positions[i - 1] < min_gap:
                    positions[i] = positions[i - 1] + min_gap

            # group consecutive short (<1 beat) melodic notes for beaming
            idx = 0
            while idx < len(events):
                e = events[idx]
                is_short = e['beats'] <= 0.5 and len(e['notes']) <= 1 and e['notes']
                if is_short:
                    group = [idx]
                    j = idx + 1
                    while j < len(events) and events[j]['beats'] <= 0.5 and \
                            len(events[j]['notes']) <= 1 and events[j]['notes']:
                        group.append(j)
                        j += 1
                    if len(group) >= 2:
                        xs = [positions[k] for k in group]
                        steps_list = [[theory.staff_step(n[0], n[2]) for n in events[k]['notes']]
                                      for k in group]
                        draw_beam_group(draw, xs, steps_list, events[group[0]]['beats'], lg, y0, layout)
                        idx = j
                        continue
                if e['notes']:
                    steps = [theory.staff_step(n[0], n[2]) for n in e['notes']]
                    draw_chord(draw, positions[idx], steps, e['beats'], lg, y0, layout)
                else:
                    draw_rest(draw, positions[idx], e['beats'], lg, y0)
                idx += 1

        final_x = content_right
        double = (sys_idx == n_systems - 1)
        top_y, bot_y = y_for_step(0, y0, lg), y_for_step(4, y0, lg)
        if double:
            draw.line([(final_x - lg * 0.22, top_y), (final_x - lg * 0.22, bot_y)], fill=WHITE, width=layout.line_width)
            draw.line([(final_x, top_y), (final_x, bot_y)], fill=WHITE, width=max(3, int(lg * 0.22)))
        else:
            draw.line([(final_x, top_y), (final_x, bot_y)], fill=WHITE, width=layout.line_width)

    img.save(out_path)
    return out_path
