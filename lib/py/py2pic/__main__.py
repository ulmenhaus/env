import glob
import os
import shutil
import subprocess
import sys

from py2pic.drawing import Drawing

WRAPPER_TEMPLATE = """
\\documentclass<article>
\\pagestyle<empty>
\\usepackage<pstricks>
\\usepackage<amsmath>
\\begin<document>
\\input<{wrapped}.tex>
\\end<document>
"""


def _convert_to_m4(draw_file):
    global draw
    meta_path = draw_file + ".meta"
    if os.path.exists(meta_path):
        with open(meta_path) as f:
            meta = f.read()
        os.environ["PIC_PY_META"] = meta
    with open(draw_file) as f:
        exec(f.read(), globals())
    d = Drawing()
    draw(d)
    return d.render()


def _fixup_tex(path):
    with open(path) as f:
        tex = f.read()
    # HACK removing the first point from any beziers seems to fix an issue
    # with filled beziers starting from the origin instead of last point
    lines = tex.split("\n")
    filled = False
    for i, line in enumerate(lines[:-1]):
        if line.startswith(r"\pscustom[fillcolor"):
            filled = True
        if filled and line.startswith(r"}%"):
            filled = False
        if filled and line.startswith(
                r"\psbezier") and not lines[i - 1].startswith(r"\pscustom"):
            lines[i + 1] = lines[i + 1].split(")", 1)[1]
    with open(path, 'w') as f:
        f.write("\n".join(lines))


def main():
    draw_file = sys.argv[1]
    name = os.path.basename(draw_file)
    if draw_file.endswith(".py"):
        name = name[:-3]
        m4 = _convert_to_m4(draw_file)
    elif draw_file.endswith(".m4"):
        name = name[:-3]
        with open(draw_file) as f:
            m4 = f.read()
    else:
        raise ValueError("I do not know how to render {}".format(name))

    if not os.path.exists("build"):
        os.mkdir("build")
    for fpath in glob.glob(os.path.join(os.path.dirname(__file__), "*.m4")):
        target = os.path.join("build", os.path.basename(fpath))
        if not os.path.exists(target):
            shutil.copyfile(fpath, target)

    os.chdir("build")

    with open("{}.m4".format(name), 'w') as f:
        f.write(m4)

    with open("{}_wrapper.tex".format(name), 'w') as f:
        f.write(
            WRAPPER_TEMPLATE.format(wrapped=name).replace("<", "{").replace(
                ">", "}"))

    cmds = [
        "rm -f {name}_wrapper.eps",
        "m4 pstricks.m4 libcct.m4 liblog.m4 libgen.m4 lib3D.m4 {name}.m4 > {name}.pic",
        "dpic -p {name}.pic > {name}.tex",
        "latex {name}_wrapper.tex",
        "dvips {name}_wrapper.dvi",
        "ps2eps {name}_wrapper.ps",
        "epspdf {name}_wrapper.eps",
        "pdf2svg {name}_wrapper.pdf {name}.svg",
    ]
    for cmd in cmds:
        subprocess.check_call(["bash", "-c", cmd.format(name=name)])
        if cmd.startswith("dpic "):
            _fixup_tex("{}.tex".format(name))


if __name__ == "__main__":
    main()
