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
\\begin<document>
\\input<{wrapped}.tex>
\\end<document>
"""


def main():
    d = Drawing()
    draw_file = sys.argv[1]
    if not draw_file.endswith(".py"):
        draw_file += ".py"
    base = os.path.basename(draw_file)
    nopy = ".".join(base.split(".")[:-1])
    global draw
    with open(draw_file) as f:
        exec(f.read(), globals())
    draw(d)
    if not os.path.exists("build"):
        os.mkdir("build")
        for fpath in glob.glob(os.path.join(os.path.dirname(__file__),
                                            "*.m4")):
            shutil.copyfile(fpath, os.path.join("build", os.path.basename(fpath)))

    os.chdir("build")

    with open("{}.m4".format(nopy), 'w') as f:
        f.write(d.render())
    with open("{}_wrapper.tex".format(nopy), 'w') as f:
        f.write(
            WRAPPER_TEMPLATE.format(wrapped=nopy).replace("<", "{").replace(
                ">", "}"))

    cmds = [
        "rm -f {nopy}_wrapper.eps",
        "m4 pstricks.m4 {nopy}.m4 > {nopy}.pic",
        "dpic -p {nopy}.pic > {nopy}.tex",
        "latex {nopy}_wrapper.tex",
        "dvips {nopy}_wrapper.dvi",
        "ps2eps {nopy}_wrapper.ps",
        "epspdf {nopy}_wrapper.eps",
        "pdf2svg {nopy}_wrapper.pdf {nopy}.svg",
    ]
    for cmd in cmds:
        subprocess.check_call(["bash", "-c", cmd.format(nopy=nopy)])


if __name__ == "__main__":
    main()
