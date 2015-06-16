import subprocess
import urllib.parse

from passtools.interface import BashInterface
from passtools.context import ContextManager

PASSWORDS = {
    'hub.docker.com': ('personal', 'docker/dockerhub'),
    'docker.atlassian.net': ('personal', 'docker/atlassian'),
}

GET_ACTIVE_PAGE_URL_SCRIPT = """
tell application "Google Chrome" to get URL of active tab of front window
set active_title to text of result
do shell script "echo " & quoted form of active_title
"""

INSERT_FROM_CLIPBOARD = """
tell application "Google Chrome"
     activate
     delay .5
     tell application "System Events"
     	  keystroke "v" using {command down}
	  end tell
end tell
"""


class ChromePasswordManager(object):
    def __init__(self, bash_interface, passwords=PASSWORDS):
        self.passwords = passwords
        self.context_manager = ContextManager(bash_interface)
        self.bash_interface = bash_interface

    def insert_password(self):
        url = urllib.parse.urlparse(self.get_active_page_url())
        context, passloc = self.passwords[url.netloc.decode('UTF-8')]
        self.context_manager.run(context, '-e')
        self.bash_interface.run("pass", "-c", passloc)
        print(self.get_osascript_output(INSERT_FROM_CLIPBOARD))

    def get_active_page_url(self):
        return self.get_osascript_output(GET_ACTIVE_PAGE_URL_SCRIPT)

    def get_osascript_output(self, input):
        proc = subprocess.Popen(["osascript"],
                                stdin=subprocess.PIPE,
                                stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE)
        stdout, stderr = proc.communicate(input=bytes(input, 'UTF-8'))
        exit_code = proc.wait()
        if exit_code:
            raise Exception(exit_code, stdout, stderr)
        return stdout


def main():
    interface = BashInterface()
    with interface.patch():
        ChromePasswordManager(interface).insert_password()


if __name__ == "__main__":
    main()
