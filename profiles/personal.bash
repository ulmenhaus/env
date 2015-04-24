export PATH=$HOME/source/caervs/personal/exe:$PATH
export EDITOR=emacs

function devmode {
    export PYTHONPATH=~/source/caervs/public/
}

# TODO reconcile all of my context logic

function pass-context {
    rm -f ~/.password-store
    context="$1"
    mode="$2"
    storepath=""

    if [ "$context" == "" ];
    then
	cat ~/.pass-context;
    fi;
    if [ "$context" == "docker" ];
    then
	storepath="$HOME/source/docker-infra/pass-store";
	gitpath="$HOME/source/docker-infra/pass-store";
    fi;
    if [ "$context" == "personal" ];
    then
	storepath="$HOME/source/caervs/private/password-store";
	gitpath="$HOME/source/caervs/private/";
    fi;
    if [ "$context" == "peripheral" ];
    then
	storepath="/Volumes/Key/password-store";
	gitpath="/Volumes/Key/password-store";
    fi;
    if [ "$context" == "test" ];
    then
	tmppath=/tmp/password-store
	mkdir -p $tmppath
	storepath=$tmppath
	gitpath=$tmppath
    fi;
    if [ "$storepath" ];
    then
	unset PASSWORD_STORE_DIR
	unset PASSWORD_STORE_GIT

	ln -s $storepath ~/.password-store
	echo $1 > ~/.pass-context
    fi;
    if [ "$mode" == "-e" ];
    then
	export PASSWORD_STORE_DIR=$storepath
	export PASSWORD_STORE_GIT=$gitpath
    fi;
}

function gpg-context {
    rm -f ~/.gnupg ~/.ssh

    context="$1"
    sshdir="$HOME/source/caervs/local/standard_context/ssh";
    if [ "$context" == "standard" ];
    then
	gpgdir="$HOME/source/caervs/local/standard_context/gnupg";
    fi;
    if [ "$context" == "setup" ];
    then
	gpgdir="$HOME/source/caervs/local/setup_context/gnupg";
    fi;
    if [ "$context" == "peripheral" ];
    then
	gpgdir="/Volumes/KEY/gnupg"
    fi;

    ln -s $gpgdir ~/.gnupg
    ln -s $sshdir ~/.ssh

}

function fab-context {
    rm -f ~/.highland
    ln -s $HOME/source/caervs/local/standard_context/fab/highland ~/.highland
}

function pass-gen-ssh-key {
    old_context=$(pass-context)
    pass-context peripheral -e
    keyname=$1
    pass generate --no-symbols rsa/$keyname 40
    ssh-keygen -f $HOME/source/caervs/local/standard_context/ssh/$keyname -P $(pass show rsa/$keyname)
    pass-context $old_context
}

function add-keypair {
    old_context=$(pass-context)
    pass-context peripheral
    keyname=$1
    ssh-add-with-password $HOME/source/caervs/local/standard_context/ssh/$keyname $(pass show rsa/$keyname)
    pass-context $old_context
}

. ~/source/zx2c4/password-store/src/completion/pass.bash-completion

if [ -f `brew --prefix`/etc/bash_completion ]; then
    . `brew --prefix`/etc/bash_completion
fi


alias blender='~/Downloads/Blender/blender.app/Contents/MacOS/blender'
