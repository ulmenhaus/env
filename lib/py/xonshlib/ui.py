import os

import xonshlib.glyphs

from xonshlib import ENV


class Prompter(object):
    def __init__(self, template, pass_context_manager, truncated_dirs=()):
        self.template = template
        self.pass_context_manager = pass_context_manager
        self.truncated_dirs = truncated_dirs

    def prompt(self):
        return self.template.format(
            pass_color="{GREEN}" if "PASSWORD_STORE_DIR" in ENV else "{RED}",
            lock_glyph=xonshlib.glyphs.Objects.LOCK,
            pass_context=self.pass_context_manager.get_context(),
            docker_color="{BLUE}",
            kube_active=(xonshlib.glyphs.Objects.HELM
                         if "KUBECONFIG" in ENV else ""),
            whale_glyph=xonshlib.glyphs.Animals.WHALE,
            docker_machine=ENV.get("DOCKER_MACHINE",
                                   xonshlib.glyphs.Words.PINATA),
            dir_color="{YELLOW}",
            short_wd=self._shorten_dir(ENV['PWD']),
            end="{RESET} {prompt_end} ", )

    def _shorten_dir(self, fulldir):
        prefixes = [
            os.path.join(trunc_dir, "") for trunc_dir in self.truncated_dirs
        ]
        for prefix in prefixes:
            if fulldir.startswith(prefix):
                return fulldir[len(prefix):]
        return fulldir
