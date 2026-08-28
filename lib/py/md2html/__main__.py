#!/usr/bin/env python3

import argparse
import pathlib
from typing import Any, Dict

import graphviz
import markdown

from markdown.extensions import codehilite
from pygments.formatters import HtmlFormatter
from pymdownx.superfences import fence_code_format

from timedb.markdown import render

def graphviz_format(source: str, language: str, class_name: str,
                    options: Dict[str, Any], md: markdown.Markdown,
                    **kwargs) -> str:
    if not _GRAPHVIZ_AVAILABLE:
        return fence_code_format(source, language, class_name, options, md,
                                 **kwargs)

    engine = options.get("engine", "dot")
    try:
        gv = graphviz.Source(source, engine=engine, format="svg")
        svg_bytes = gv.pipe(format="svg")
        svg = svg_bytes.decode("utf-8")
        return svg
    except Exception:
        return fence_code_format(source, language, class_name, options, md,
                                 **kwargs)



def build_markdown() -> markdown.Markdown:
    md = markdown.Markdown(
        extensions=[
            "toc",
            "tables",
            "fenced_code",
            "attr_list",
            "admonition",
            "md_in_html",
            "pymdownx.arithmatex",
            "pymdownx.betterem",
            "pymdownx.caret",
            "pymdownx.details",
            "pymdownx.emoji",
            "pymdownx.highlight",  # Extra code highlighting features
            "pymdownx.inlinehilite",  # `==inline code==` style hilite
            "pymdownx.keys",
            "pymdownx.magiclink",
            "pymdownx.mark",
            "pymdownx.smartsymbols",
            "pymdownx.superfences",
            "pymdownx.tasklist",
        ],
        extension_configs={
            "pymdownx.arithmatex": {
                "generic": True
            },
            "pymdownx.superfences": {
                "custom_fences": [{
                    "name": "graphviz",
                    "class": "graphviz",
                    "format": graphviz_format
                }, {
                    "name": "dot",
                    "class": "graphviz",
                    "format": graphviz_format
                }]
            },
            "pymdownx.tasklist": {
                "custom_checkbox": True
            },
            "pymdownx.highlight": {
                "use_pygments": False,
                "guess_lang": False
            },
            "toc": {
                "permalink": "", 
                "permalink_class": "headerlink",
                "toc_depth": "2-6",
            },
        })
    return md


def render_html(markdown_text: str, title: str, template: str, disable_toc: bool) -> str:
    md = build_markdown()
    html_body = md.convert(markdown_text)
    toc = "" if disable_toc else md.toc
    doc = template.replace("{{title}}", title).replace("{{toc}}", toc).replace("{{content}}", html_body)
    return doc


def main() -> None:
    parser = argparse.ArgumentParser(
        description=
        "Render Markdown to HTML with pymdown-extensions, MathJax, Pygments, and Graphviz."
    )
    parser.add_argument("input", type=pathlib.Path, help="Input Markdown file")
    parser.add_argument("template", type=pathlib.Path, help="Input template file")
    parser.add_argument("title", type=str, help="Page title")
    parser.add_argument("--disable-toc", action="store_true", help="Disable the table of contents")
    parser.add_argument(
        "-o",
        "--output",
        type=pathlib.Path,
        default=None,
        help="Output HTML file (defaults to input name with .html)")
    args = parser.parse_args()

    if not args.input.exists():
        raise SystemExit(f"Input file not found: {args.input}")

    text = args.input.read_text(encoding="utf-8")
    title = args.title
    markdown = render.inject_externals("tls", text)
    html = render_html(markdown, title=title, template=args.template.read_text("utf-8"), disable_toc=args.disable_toc)

    out_path = args.output or args.input.with_suffix(".html")
    out_path.write_text(html, encoding="utf-8")
    print(f"Wrote {out_path}")


if __name__ == "__main__":
    main()
