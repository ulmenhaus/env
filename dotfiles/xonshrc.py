import os

import xonshlib.ui
import xonshlib.utils

ENV = __xonsh_env__

ENV['XONSH_SHOW_TRACEBACK'] = True

HOME = ENV["HOME"]
GH = os.path.join(HOME, "src", "github.com")
GH_ALIAS = "caervs"

PASS_STORES = {
    "personal": (os.path.join(GH, GH_ALIAS, "private"), "password-store"),
    "docker": os.path.join(GH, "docker-infra", "pass-store"),
    "uhaus": (os.path.join(GH, "ulmenhaus", "private"), "password-store"),
}

PASS_MANAGER = xonshlib.utils.PasswordManager(PASS_STORES)
PASS_COMPLETE = PASS_MANAGER.complete_line

aliases.update({
    'pc': PASS_MANAGER.set_context,
    'pass-env': PASS_MANAGER.get_env,
    'pass-add-key': PASS_MANAGER.add_ssh_key,
    'pass-submit': PASS_MANAGER.submit_data,
    'dm': xonshlib.utils.docker_machine_env,
    'set_completers': "completer add pass PASS_COMPLETE start",
    # TODO make src parametrically take in a project name
    'src': 'cd ~/src/github.com/',
    'docker-clean': xonshlib.utils.docker_clean,
    'git-clean': xonshlib.utils.git_clean,
})

PROMPT_TEMPLATE = "{pass_color}{lock_glyph} {pass_context} \
{docker_color}{whale_glyph} {docker_machine} {dir_color}{short_wd}{end}"

ENV['PROMPT'] = xonshlib.ui.Prompter(PROMPT_TEMPLATE, PASS_MANAGER, [GH]).prompt

# TODO would be good to separate work related and personal configs
DOCKER_CONFIGS = {
    'highland_dev': {
        'image': 'highland_dev',
        'it': True,
        'rm': True,
        'net': 'host',
        'privileged': True,
        'volumes': {
            "/var/run/docker.sock": "/var/run/docker.sock ",
            "~/.gnupg-root": "/root/.gpg ",
            "~/.dockercfg": "/root/.dockercfg ",
            "~/src/github.com/docker-infra/pass-store":
            "/root/.password-store ",
            "~/src/github.com/docker/highland/": "/highland/ ",
            "~/src/github.com/docker/saas-mega/": "/saas-mega/ ",
            "/Users/rabrams": "/rabrams ",
            "~/src/github.com/docker-infra/docker-ca": "/dockerca",
        }
    }
}

aliases.update({alias: xonshlib.utils.command_for_container_config(config)
                for alias, config in DOCKER_CONFIGS.items()})

set_completers
clear
