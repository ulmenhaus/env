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
    with open(draw_file) as f:
        exec(f.read(), globals())
    d = Drawing()
    draw(d)
    return d.render()


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
        for fpath in glob.glob(os.path.join(os.path.dirname(__file__),
                                            "*.m4")):
            shutil.copyfile(fpath,
                            os.path.join("build", os.path.basename(fpath)))

    os.chdir("build")

    with open("{}.m4".format(name), 'w') as f:
        f.write(m4)

    with open("{}_wrapper.tex".format(name), 'w') as f:
        f.write(
            WRAPPER_TEMPLATE.format(wrapped=name).replace("<", "{").replace(
                ">", "}"))

    cmds = [
        "rm -f {name}_wrapper.eps",
        "m4 pstricks.m4 {name}.m4 > {name}.pic",
        "dpic -p {name}.pic > {name}.tex",
        "latex {name}_wrapper.tex",
        "dvips {name}_wrapper.dvi",
        "ps2eps {name}_wrapper.ps",
        "epspdf {name}_wrapper.eps",
        "pdf2svg {name}_wrapper.pdf {name}.svg",
    ]
    for cmd in cmds:
        subprocess.check_call(["bash", "-c", cmd.format(name=name)])


if __name__ == "__main__":
    main()
