import os

try:
    ENV = __xonsh__.env
except NameError:
    ENV = os.environ
