import html
import os

from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import urlparse, parse_qs

HOST_NAME = 'localhost'
PORT_NUMBER = 9070
DAEMON_BIND_ADDRESS = os.getenv("JQL_MD_BIND_ADDRESS") or "localhost"


class MarkdownDaemon(BaseHTTPRequestHandler):

    def __init__(self, template):
        MarkdownDaemonHandler.template = template

    def serve_forever(self):
        httpd = HTTPServer((DAEMON_BIND_ADDRESS, PORT_NUMBER),
                           MarkdownDaemonHandler)
        httpd.serve_forever()


class MarkdownDaemonHandler(BaseHTTPRequestHandler):
    template = None

    def do_HEAD(self):
        self.send_response(200)
        self.send_header("Content-type", "text/html")
        self.end_headers()

    def do_GET(self):
        contents = self._render()
        self.send_response(200)
        self.send_header("Content-type", "text/html")
        self.end_headers()
        self.wfile.write(contents.encode("utf-8"))

    def _render(self):
        query_components = parse_qs(urlparse(self.path).query)
        markdown = bytes.fromhex(query_components.get("raw", [""])[0]).decode("utf-8")
        body = self.template.replace("{md-contents}", html.escape(markdown))
        return body.replace("{md-postfix}", "").replace("{md-prefix}", "")
