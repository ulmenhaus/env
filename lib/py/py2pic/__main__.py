import os
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
    global draw
    with open(draw_file) as f:
        exec(f.read(), globals())
    draw(d)
    base = os.path.basename(draw_file)
    nopy = ".".join(base.split(".")[:-1])
    with open("{}.m4".format(nopy), 'w') as f:
        f.write(d.render())
    with open("{}_wrapper.tex".format(nopy), 'w') as f:
        f.write(
            WRAPPER_TEMPLATE.format(wrapped=nopy).replace("<", "{").replace(
                ">", "}"))


if __name__ == "__main__":
    main()
