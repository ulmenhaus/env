function devmode {
    export PYTHONPATH=~/source/caervs/public/
}

# TODO reconcile all of my context logic

function pass-context {
    rm -f ~/.password-store
    if [ $1 == "docker" ];
    then
	storepath="$HOME/source/docker-infra/pass-store";
	unset PASSWORD_STORE_GIT;
    fi;
    if [ $1 == "personal" ];
    then
	storepath="$HOME/source/caervs/private/password-store";
	export PASSWORD_STORE_GIT="$HOME/source/caervs/private/";
    fi;
    export PASSWORD_STORE_DIR=$storepath
    ln -s $storepath ~/.password-store
}

function gpg-context {
    rm -rf ~/.gnupg
    rm -rf ~/.ssh
    if [ $1 == "standard" ];
    then
	gpdir="$HOME/source/caervs/local/standard_context/gnupg";
	sshdir="$HOME/source/caervs/local/standard_context/ssh";
    fi;
    if [ $1 == "setup" ];
    then
	gpdir="$HOME/source/caervs/local/setup_context/gnupg";
	sshdir="$HOME/source/caervs/local/standard_context/ssh";
    fi;
    ln -s $gpdir ~/.gnupg
    ln -s $sshdir ~/.ssh
}

function fab-context {
    rm -f ~/.highland
    ln -s $HOME/source/caervs/local/standard_context/fab/highland ~/.highland
}

. ~/source/zx2c4/password-store/src/completion/pass.bash-completion

if [ -f `brew --prefix`/etc/bash_completion ]; then
    . `brew --prefix`/etc/bash_completion
fi
