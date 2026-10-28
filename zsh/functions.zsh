#!/usr/bin/env zsh

#############################################################################
### FUNCTIONS
#############################################################################

## AWS CLI

# LAMBDA UPDATE FUNCTION CODE
aws-lambda-update() {
	aws lambda update-function-code --function-name $1 --s3-bucket $2 --s3-key $3 --profile $4
}

# Create a new directory and enter it
mkd() {
	mkdir -pv -- "$1" &&
		cd -P -- "$1"
}
# create directory
mkf() {
	mkdir -pv -- "$1"
}

# CD Into Things
cd_go_proj() {
	cd "$HOME/go/src/github.com/ipyandy/$1"
}

# find shorthand
f() {
	find . -name "$1"
}

# cd into whatever is the forefront Finder window.
cdf() { # short for cdfinder
	cd "$(osascript -e 'tell app "Finder" to POSIX path of (insertion location as alias)')"
}

# Nix-Darwain Aliases
function nix-rebuild() {
  darwin-rebuild switch --flake "${HOME}"/.config/dotfiles/nix-darwin#"${1:-${HOSTNAME}}"
}

# TERRAFORM
tf-plan() {
	# terraform plan -out plan.tfplan >plan.txt | tee
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
reset-kubeconfig() {
	source "${HOME}/.bin/reset-kubeconfig.sh"
}

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
cp_ssh() {
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

# Test if HTTP compression (RFC 2616 + SDCH) is enabled for a given URL.
# Send a fake UA string for sites that sniff it instead of using the Accept-Encoding header. (Looking at you, ajax.googleapis.com!)
httpcompression() {
	encoding="$(curl -LIs -H 'User-Agent: Mozilla/5 Gecko' -H 'Accept-Encoding: gzip,deflate,compress,sdch' "$1" | grep '^Content-Encoding:')" && echo "$1 is encoded using ${encoding#* }" || echo "$1 is not using any encoding"
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

# docker compose up - 1Password integration
op-dc-up() {
	op run --no-masking -- docker compose up -d --remove-orphans
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

# Syntax-highlight JSON strings or files
json() {
	if [ -p /dev/stdin ]; then
		# piping, e.g. `echo '{"foo":42}' | json`
		python -mjson.tool | pygmentize -l javascript
	else
		# e.g. `json '{"foo":42}'`
		python -mjson.tool <<<"$*" | pygmentize -l javascript
	fi
}

# take this repo and copy it to somewhere else minus the .git stuff.
gitexport() {
	mkdir -p "$1"
	git archive master | tar -x -C "$1"
}

# get gzipped size
gz() {
	echo "orig size    (bytes): "
	cat "$1" | wc -c
	echo "gzipped size (bytes): "
	gzip -c "$1" | wc -c
}

# All the dig info
digga() {
	dig +nocmd "$1" any +multiline +noall +answer
}

# Escape UTF-8 characters into their 3-byte format
escape() {
	printf "\\\x%s" $(printf "$@" | xxd -p -c1 -u)
	echo # newline
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
