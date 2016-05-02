function work-context {
    # only highland is supported for now
    cd ~/source/docker/highland
    source venv/bin/activate
}

function set-mail-shortcuts {
    defaults write com.apple.mail NSUserKeyEquivalents '{"Office Notifications"="^o"; "Company Notifications"="^c"; "Birthdays"="^b"; "Welcomes"="^w";}'; killall Mail; open /Applications/Mail.app
}

function highland_dev {
    docker run --rm -it --net host -v /var/run/docker.sock:/var/run/docker.sock --privileged -v ~/.gnupg-root:/root/.gnupg -v ~/.dockercfg:/root/.dockercfg -v ~/source/docker-infra/pass-store:/root/.password-store -v ~/source/docker/highland/:/highland/ -v /Users/rabrams:/rabrams highland_dev
}

function prod-ak {
    password=$(pass show docker/dockerhub)
    token=$(curl -X POST -H "content-type: application/json" -d "{\"username\": \"rabrams\", \"password\": \"$password\"}" https://hub.docker.com/v2/users/login/ | jq -r .token)
    accesskey=$(curl -X POST -H "Authorization: JWT $token" https://hub.docker.com/v2/users/rabrams/accesskeys/ | jq .secret)
    echo "export ACCESSKEY=$accesskey" | pbcopy
}

function prod-ak-highland {
    password=$(pass show dev/teams/highland/production/hub/pass)
    token=$(curl -X POST -H "content-type: application/json" -d "{\"username\": \"highland\", \"password\": \"$password\"}" https://hub.docker.com/v2/users/login/ | jq -r .token)
    accesskey=$(curl -X POST -H "Authorization: JWT $token" https://hub.docker.com/v2/users/highland/accesskeys/ | jq .secret)
    echo "export ACCESSKEY=$accesskey" | pbcopy
}

function stage-ak {
    password=$(pass show docker/dockerhub-staging)
    token=$(curl -X POST -H "content-type: application/json" -d "{\"username\": \"rabrams\", \"password\": \"$password\"}" https://hub-stage.docker.com/v2/users/login/ | jq -r .token)
    accesskey=$(curl -X POST -H "Authorization: JWT $token" https://hub-stage.docker.com/v2/users/rabrams/accesskeys/ | jq .secret)
    echo "export ACCESSKEY=$accesskey" | pbcopy
}

function stage-ak-highland {
    password=$(pass show dev/teams/highland/staging/hub/pass)
    token=$(curl -X POST -H "content-type: application/json" -d "{\"username\": \"highland\", \"password\": \"$password\"}" https://hub-stage.docker.com/v2/users/login/ | jq -r .token)
    accesskey=$(curl -X POST -H "Authorization: JWT $token" https://hub-stage.docker.com/v2/users/highland/accesskeys/ | jq .secret)
    echo "export ACCESSKEY=$accesskey" | pbcopy
}
