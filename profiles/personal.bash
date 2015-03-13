function devmode {
    export PYTHONPATH=~/source/caervs/public/
}

function pass-context {
    rm -f ~/.password-store
    if [ $1 == "docker" ];
    then
	storepath="/Users/caervs/source/docker-infra/pass-store";
	unset PASSWORD_STORE_GIT;
    fi;
    if [ $1 == "personal" ];
    then
	storepath="/Users/caervs/source/caervs/private/password-store";
	export PASSWORD_STORE_GIT="/Users/caervs/source/caervs/private/";
    fi;
    ln -s $storepath ~/.password-store
}

. ~/source/zx2c4/password-store/src/completion/pass.bash-completion

if [ -f `brew --prefix`/etc/bash_completion ]; then
    . `brew --prefix`/etc/bash_completion
fi
