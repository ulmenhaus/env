import builtins
import contextlib
import functools


class BashEnvironmentInterface(object):
    def __init__(self, bash_interface, exports=True):
        self.bash_interface = bash_interface
        self.exports = exports

    def __setitem__(self, key, value):
        export_string = "export" if self.exports else ""
        output = "%s %s=%s\n" % (export_string, key, value)
        self.bash_interface.real_print(output)

    def __delitem__(self, key):
        output = "unset %s" % key
        self.bash_interface.real_print(output)
        


class BashInterface(object):
    def __init__(self):
        self.exported_env = BashEnvironmentInterface(self)

    @contextlib.contextmanager
    def patch(self):
        self.real_print = builtins.print
        builtins.print = self.wrapped_print
        yield
        builtins.print = self.real_print

    def wrapped_print(self, *o):
        self.real_print("echo", *o)

    def run(self, *args):
        self.real_print(" ".join(args))

    def __getattr__(self, cmd):
        return functools.partial(self.run, cmd)


