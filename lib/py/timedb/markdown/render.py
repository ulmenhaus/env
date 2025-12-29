import hashlib
import os
import schemdraw
import subprocess
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


def render_pics(grouped, meta):
    for i, group in enumerate(grouped):
        lines = group.split("\n")
        if lines[0] not in ["```pic.py", "```m4", "```plt.py", "```schem.py"]:
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
        yield render_pic(src)


def inject_externals(default_project, markdown):
    suffix_to_language = {
        "py": "python",
        "v": "verilog",
    }
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

    final = []
    meta = {}
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

            for arg in args[1:]:
                if match := re.match('until="(?P<until>.*)"\Z', arg):
                    until = match.group("until")
                elif match := re.match('after="(?P<after>.*)"\Z', arg):
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

    final = render_pics(final, meta)
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
        parts.append(_render_link(ref, "/nouns " + ref))
        parts.append(rest)
    return "".join(parts)
