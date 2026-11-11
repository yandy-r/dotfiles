#!/usr/bin/env zsh

#############################################################################
### FUNCTIONS
#############################################################################

## AWS CLI

# LAMBDA UPDATE FUNCTION CODE
aws-lambda-update() {
	aws lambda update-function-code --function-name $1 --s3-bucket $2 --s3-key $3 --profile $4
}

# CD Into Things
cd_go_proj() {
	cd "$HOME/go/src/github.com/ipyandy/$1"
}

# find shorthand
f() {
	find . -name "$1"
}

# TERRAFORM
tf-plan() {
	terraform plan -out plan.tfplan
}

tf-apply() {
	terraform apply plan.tfplan
}
tf-refresh() {
	terraform refresh
}
tf-m-out() {
	terraform output -module=$1 $2
}
tf-output() {
	terraform output $1
}
tf-destroy() {
	terraform apply -destroy -auto-approve
}

# KUBERNETES
get-eks-dashboard-token() {
	local secret_name=$(kubectl -n kube-system get secret | grep eks-admin | awk '{print $1}')
	[[ -n "$secret_name" ]] && kubectl -n kube-system describe secret "$secret_name"
}

eks-bastion-ssh() {
	local bastion_ip=$(terraform output -module=aws-k8s-cluster aws_bastion_pub_ip)
	[[ -n "$bastion_ip" ]] && ssh -i "$HOME/.aws-keys/aws-dev-key" ec2-user@"$bastion_ip"
}

# Azure

az-get-aks-creds() {
	command az aks get-credentials --resource-group $1 --name $2
}

# RSYNC

rsync-ssh-win() {
	command rsync -tvau -h --modify-window=10 --exclude .DS_Store --exclude node_modules --progress -e ssh $1 $2
}

rsync-win() {
	command rsync -tvau -h --modify-window=10 --exclude .DS_Store --exclude node_modules --progress $1 $2
}

rsync_ssh() {
	command rsync -tvau -h --exclude .DS_Store --exclude node_modules --progress -e ssh $1 $2
}
# copies over ssh and deletes source files
cpd_ssh() {
	command rsync -tvau -h --remove-source-files --exclude .DS_Store --exclude node_modules --progress -e ssh $1 $2
}

# Copy w/ progress
cp_p() {
	command rsync -tvau -h --exclude .DS_Store --exclude node_modules --progress $1 $2
}
# Copy w/ progress deletes source
cpd_p() {
	command rsync -tvau -h --remove-source-files --exclude .DS_Store --exclude node_modules --progress $1 $2
}

# Copy with RSYNC daemon instead of mounted or ssh
# don't delete source files
rcp() {
	command rsync -tvau -h --exclude .DS_Store --exclude node_modules --progress $1 rsync://$2
}
# delete source files
rcpd() {
	command rsync -tvau -h --remove-source-files --exclude .DS_Store --exclude node_modules --progress $1 rsync://$2
}

# DOCKER
#
# docker compose restart app
dc-restart() {
	docker compose restart $1
}

# force restart (remove and start)
dc-frestart() {
	docker compose rm -s -f $1
	docker compose up -d $1
}

# docker compose restart app
dc-start() {
	docker compose start $1
}

# docker compose stop app
dc-stop() {
	docker compose stop $1
}

# docker compose rm all
dc-rm() {
	docker compose rm -s -f -v $1
}

# docker compose pull all images
dc-pull-all() {
	docker compose pull
}

# docker compose pull specific image
dc-pull() {
	sudo docker compose pull $1
}

# docker compose up
dc-up() {
	docker compose up -d --remove-orphans
}

# docker compose down
dc-down() {
	docker compose down --remove-orphans
	docker network prune -f
}

# docker stop all
docker-stop-all() {
	local containers=$(docker ps -q)
	[[ -n "$containers" ]] && sudo docker container stop $containers
}

# docker delete all images
d-rmi() {
	sudo docker image prune -a -f
}

#docker delete all containers
docker-rm-containers() {
	local containers=$(docker ps -a -q)
	[[ -n "$containers" ]] && sudo docker container rm $containers
}

# All the dig info
digga() {
	dig +nocmd "$1" any +multiline +noall +answer
}

# Decode \x{ABCD}-style Unicode escape sequences
unidecode() {
	perl -e "binmode(STDOUT, ':utf8'); print \"$@\""
	echo # newline
}

# Extract archives - use: extract <file>
# Based on http://dotfiles.org/~pseup/.bashrc
extract() {
	if [ -f "$1" ]; then
		local filename=$(basename "$1")
		local foldername="${filename%%.*}"
		local fullpath=$(perl -e 'use Cwd "abs_path";print abs_path(shift)' "$1")
		local didfolderexist=false
		if [ -d "$foldername" ]; then
			didfolderexist=true
			read -p "$foldername already exists, do you want to overwrite it? (y/n) " -n 1
			echo
			if [[ $REPLY =~ ^[Nn]$ ]]; then
				return
			fi
		fi
		mkdir -p "$foldername" && cd "$foldername"
		case $1 in
		*.tar.bz2) tar xjf "$fullpath" ;;
		*.tar.gz) tar xzf "$fullpath" ;;
		*.tar.xz) tar Jxvf "$fullpath" ;;
		*.tar.Z) tar xzf "$fullpath" ;;
		*.tar) tar xf "$fullpath" ;;
		*.taz) tar xzf "$fullpath" ;;
		*.tb2) tar xjf "$fullpath" ;;
		*.tbz) tar xjf "$fullpath" ;;
		*.tbz2) tar xjf "$fullpath" ;;
		*.tgz) tar xzf "$fullpath" ;;
		*.txz) tar Jxvf "$fullpath" ;;
		*.zip) unzip "$fullpath" ;;
		*) echo "'$1' cannot be extracted via extract()" && cd .. && ! $didfolderexist && rm -r "$foldername" ;;
		esac
	else
		echo "'$1' is not a valid file"
	fi
}

cisco_type5() {
  openssl passwd -salt `openssl rand -base64 3` -1
}

#############################################################################
### Downloads and Miscellaneous
#############################################################################

tdl() {
  tidal-dl-ng dl $1
}

#############################################################################
### END FUNCTIONS
#############################################################################
