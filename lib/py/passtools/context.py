import sys

from passtools.interface import BashInterface


CONTEXTS = {
    'docker': ("$HOME/src/github.com/docker-infra/pass-store",
               "$HOME/src/github.com/docker-infra/pass-store"),
    'personal': ("$HOME/src/github.com/caervs/private/password-store",
                 "$HOME/src/github.com/caervs/private/"),
    'peripheral': ("/Volumes/Key/password-store",
                   "/Volumes/Key/password-store"),
    }

class ContextManager(object):
    def __init__(self, bash_interface, contexts=CONTEXTS):
        self.exports = bash_interface.exported_env
        self.bash = bash_interface
        self.contexts = contexts

    def run(self, context='', mode=''):
        if not context:
            self.bash.cat("~/.pass-context")
            exit()
        elif context not in self.contexts:
            print("Context %r does not exist" % context)
            exit()

        storepath, gitpath = self.contexts[context]

        if mode == '-e':
            self.exports["PASSWORD_STORE_DIR"] = storepath
            self.exports["PASSWORD_STORE_GIT"] = gitpath
        else:
            del self.exports["PASSWORD_STORE_DIR"]
            del self.exports["PASSWORD_STORE_GIT"]
            self.bash.rm("-f", "~/.password-store")
            self.bash.ln("-s", storepath, "~/.password-store")
            self.bash.echo(context, ">", "~/.pass-context")


def main():
    interface = BashInterface()
    with interface.patch():
        ContextManager(interface).run(*sys.argv[1:])


if __name__ == "__main__":
    main()
