import os

try:
    ENV = __xonsh_env__
except NameError:
    ENV = os.environ
