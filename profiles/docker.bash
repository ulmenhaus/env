function start-workday {
    keypairs-add
    open $PRIVATE_DIR/kdbs/master.kdbx
    for app in Google\ Chrome Mail Slack Spotify
    do
	open "/Applications/$app.app";
    done
}

function keypairs-add {
    add-keypair docker.cluster docker/rsa/cluster
}

function work-context {
    # only highland is supported for now
    cd ~/source/docker/highland
    source venv/bin/activate
}

function docker-enter {
    docker exec -it $1 bash
}

function highland-build-and-tag {
    docker build -t docker/highland:staging .
    docker tag -f docker/highland:staging highland:latest
}

function highland-build-tag-and-run {
    highland-build-and-tag
    docker-compose up
}

function test-context {
    export PYTHONPATH="/Users/caervs/source/org/docker/highland/image/:/Users/caervs/source/org/docker/highland/bootstrap/:$PYTHONPATH"
    alias fablib-cli='python -m fablib.cli --highland_password fPMDBvHffPMDBvHf'
}

function build-yoyobrawlah {
    # fablib-cli build --source_url https://github.com/caervs/yoyobrawlah --docker_repo caervs --docker_tag caervs/yoyobrawlah
    fab cli_build:https://github.com/caervs/yoyobrawlah,caervs/yoyobrawlah,latest
}

function docker-daemon-connect {
    eval $(docker-machine env osxdock)
}
# expose port 80 on docker VM with
# boot2docker ssh -L 8000:localhost:80

# to clear sql database
# docker rm db
# may have to run docker-compose stop first
# and perhaps docker rm highland_redis_1

# get redis IP
# API=$(docker ps | grep api | cut -f 1 -d ' '); docker exec $API cat /etc/hosts | grep redis

# queue new job with
# fab cli_build:https://github.com/caervs/yoyobrawlah,caervs,caervs/yoyobrawlah

# run drain worker locally with
# SETTINGS_FLAVOR=development REDIS_1_PORT_6379_TCP_ADDR=localhost REDIS_1_PORT_6379_TCP_PORT=6379 fab drain_worker


# run highland tests in a container
# docker run -it -v `pwd`/image:/image docker/highland:staging  bash -c "cd /image; trial highland"

# add ssh keys to a linode instance
# fab h:nj-a saltcall:ssh.init.add_company_ssh_keys

# push local code to staging node, build docker image and add to registry
# fab h:kernel-builder project build push:staging
# pull new highland from registry
# fab h:test-1 pull:staging worker api manager

function set-mail-shortcuts {
    defaults write com.apple.mail NSUserKeyEquivalents '{"Office Notifications"="^o"; "Company Notifications"="^c"; "Birthdays"="^b"; "Welcomes"="^w";}'; killall Mail; open /Applications/Mail.app
}

function highland_dev {
    # docker run -it -v ~/source/docker/highland:/highland -v ~/.gnupg:/root/.gnupg -v ~/.password-store:/root/.password-store -v /var/run/docker.sock:/var/run/docker.sock --privileged -v /usr/local/bin/docker:/usr/local/bin/docker -v ~/.ssh:/root/.ssh -v ~/.dockercfg:/root/.dockercfg -v ~/.highland-certs/:/root/.highland-certs highland_dev
    docker run -it --net host -v /var/run/docker.sock:/var/run/docker.sock --privileged -v /usr/local/bin/docker:/usr/local/bin/docker -v /Users/rabrams:/root -v /Users/rabrams:/rabrams -v /Users/rabrams/source/docker/highland/:/highland/ highland_dev
}


function prod-ak {
    token=$(curl -X POST -H "Authorization: authtoken $(pass show dev/teams/hub/apps/index/prod/dockerio/notification/secret)" https://hub.docker.com/v2/users/rabrams/accesskey/ | jq .secret)
    echo "export ACCESSKEY=$token" | pbcopy
}

function stage-ak {
    token=$(curl -X POST -H "Authorization: authtoken $(pass show dev/teams/hub/apps/index/stage/dockerio/token)" https://hub-stage.docker.com/v2/users/rabrams/accesskey/ | jq .secret)
    echo "export ACCESSKEY=$token" | pbcopy
}
