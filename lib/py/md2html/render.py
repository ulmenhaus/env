import hashlib
import importlib.util
import os
import re
import schemdraw
import subprocess
import sys
import time

import schemdraw.elements as elm

def watch_for_update(filepath, poll_interval=2):
    update_time = None
    while True:
        time.sleep(poll_interval)
        try:
            new_time = os.stat(filepath).st_mtime
        except FileNotFoundError:
            continue
        if new_time != update_time:
            update_time = new_time
            # HACK might still be writing the file so sleep for another 100ms
            time.sleep(.1)
            yield


def render_pic(src):
    wd = os.getcwd()
    target = "{}.svg".format(src[:-3])
    if not os.path.exists(target):
        if src.endswith(".schem.py"):
            # Use path-based svg rendering of text to support math expressions
            schemdraw.svgconfig.text = 'path'
            # TODO if there's an exception then remove the svg file
            with schemdraw.Drawing(file=target, show=False, lw=.8) as d:
                global draw
                meta_path = src + ".meta"
                if os.path.exists(meta_path):
                    with open(meta_path) as f:
                        meta = f.read()
                with open(src) as f:
                    exec(f.read(), globals())
                draw(d, elm, meta)
        else:
            subprocess.check_call([
                "docker",
                "run",
                "-i",
                "--rm",
                "--entrypoint=pp",
                "-w",
                wd,
                "-v",
                "{}:{}".format(os.getenv("JQL_HOST_MOUNT_PREFIX") + wd, wd),
                "ulmenhaus/env",
                src,
            ])
    img = '<div style="text-align:center; padding-bottom: 25px;"><img src="{}" /></div>\n'.format(
        target)
    return img


class Context(object):
    """Context passed to a widget's render function. Currently bare-bones."""


class Output(object):
    """Collects the html, js, and css produced by a widget's render function."""

    def __init__(self):
        self.html = ""
        self.js = ""
        self.css = ""

    def set_html(self, html):
        self.html = html

    def set_js(self, js):
        self.js = js

    def set_css(self, css):
        self.css = css


def _unload_modules_from(directory):
    """Removes modules loaded from directory out of sys.modules so that widgets in
    different directories with same-named helpers don't collide."""
    prefix = os.path.join(directory, "")
    for name, module in list(sys.modules.items()):
        path = getattr(module, "__file__", None)
        if path and os.path.abspath(path).startswith(prefix):
            del sys.modules[name]


def render_widget(src, src_dir=None):
    """Loads a .widget.py module, calls its render(ctx, out) function, and returns the
    resulting html, css, and js as a raw html block. If src_dir is provided it is added
    to the import path so the widget can import modules that live alongside it."""
    module_name = "md2html_widget_{}".format(
        os.path.basename(src).split(".", 1)[0])
    spec = importlib.util.spec_from_file_location(module_name, src)
    module = importlib.util.module_from_spec(spec)
    out = Output()
    if src_dir:
        src_dir = os.path.abspath(src_dir)
        sys.path.insert(0, src_dir)
    try:
        spec.loader.exec_module(module)
        module.render(Context(), out)
    finally:
        if src_dir:
            sys.path.remove(src_dir)
            _unload_modules_from(src_dir)
    parts = ['<div class="widget">']
    if out.css:
        parts.append("<style>\n{}\n</style>".format(out.css))
    if out.html:
        parts.append(out.html)
    if out.js:
        parts.append("<script>\n{}\n</script>".format(out.js))
    parts.append("</div>")
    # Surround with blank lines so markdown treats this as a raw html block
    return "\n" + "\n".join(parts) + "\n"


def render_pics(grouped, meta, src_dirs=None):
    src_dirs = src_dirs or {}
    os.makedirs("build", exist_ok=True)
    for i, group in enumerate(grouped):
        lines = group.split("\n")
        if lines[0] not in ["```pic.py", "```m4", "```plt.py", "```schem.py", "```widget.py"]:
            yield group
            continue
        group_meta = meta.get(i, "").encode("utf-8")
        suffix = lines[0][3:]
        body = "\n".join(lines[1:-1]).encode("utf-8")
        sha = hashlib.sha256()
        sha.update(body)
        sha.update(b"\x00")
        sha.update(group_meta)
        dgst = sha.hexdigest()[:8]
        src = "build/{}.{}".format(dgst, suffix)
        meta_src = src + ".meta"
        if not os.path.exists(src):
            with open(src, 'wb') as f:
                f.write(body)
            with open(meta_src, 'wb') as f:
                f.write(group_meta)
        if suffix == "widget.py":
            yield render_widget(src, src_dirs.get(i))
        else:
            yield render_pic(src)


def render_link(description, path):
    return f"[{description}]({path})"

def group_fences(markdown):
    """Splits markdown into groups where each fenced code block is a single group and
    every other line is its own group."""
    lines = markdown.split("\n")
    grouped = []
    while lines:
        first = lines.pop(0)
        if first.startswith("```"):
            while lines:
                to_add = lines.pop(0)
                first += "\n" + to_add
                if to_add.startswith("```"):
                    break
        grouped.append(first)
    return grouped


def render_diagrams(markdown):
    """Renders diagram fences (pic.py, m4, plt.py, schem.py) to images and widget fences
    (widget.py) to inline html, leaving the rest of the markdown untouched. Rendered sources and images are stored under build/."""
    return "\n".join(render_pics(group_fences(markdown), {}))


def inject_externals(default_project, markdown):
    suffix_to_language = {
        "py": "python",
        "v": "verilog",
    }
    grouped = group_fences(markdown)

    final = []
    meta = {}
    src_dirs = {}
    for i, group in enumerate(grouped):
        if group.startswith("```external"):
            lines = group.split("\n")
            meta[i] = "\n".join(lines[1:-1])
            args = lines[0].split(" ")[1:]
            filename = args[0]
            until = None
            after = None
            if ":" in filename:
                filename, section = filename.split(":")
                comment_prefix = _comment_prefix(filename)
                after = f"{comment_prefix} start_section: {section}"
                until = f"{comment_prefix} end_section: {section}"
            suffix = filename.split(".", 1)[1]
            if "/" not in filename:
                filename = "{}/{}".format(default_project, filename)
            filename = "snippets/{}".format(filename)
            if suffix == "widget.py":
                src_dirs[i] = os.path.dirname(filename)

            for arg in args[1:]:
                if match := re.match(r'until="(?P<until>.*)"\Z', arg):
                    until = match.group("until")
                elif match := re.match(r'after="(?P<after>.*)"\Z', arg):
                    after = match.group("after")
            with open(filename) as f:
                body = f.read()
                if after:
                    body = body.split(after, 1)[1]
                if until:
                    body = body.split(until, 1)[0]
            group = "```{}\n{}\n```".format(
                suffix_to_language.get(suffix, suffix), body)
        final.append(group)

    final = render_pics(final, meta, src_dirs)
    return resolve_jql_links("\n".join(final))


def resolve_jql_links(base_markdown):
    inputs = base_markdown.split("@{nouns ")
    parts = []
    for i, inpt in enumerate(inputs):
        if i == 0:
            parts.append(inpt)
            continue
        # technically disallows to link with a noun that has a colon but shrug
        try:
            ref, rest = inpt.split(":", 1)
        except:
            raise Exception("Exception breaking line", inpt)
        parts.append(render_link(ref, "/nouns " + ref))
        parts.append(rest)
    return "".join(parts)
