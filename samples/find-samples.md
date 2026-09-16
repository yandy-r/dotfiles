# File Management and Media Processing Examples

This file contains example commands for file management, permission handling, and media processing using modern CLI tools like `fd` and `ffmpeg`.

## Set Permissions

### Change Owner

- Change ownership of both user and group

```bash
fd . /mnt/nas01-data --type f --owner yandy -X sudo chown yandy:users
fd . /mnt/nas02-data --type f --owner yandy -X sudo chown yandy:users
fd . /mnt/nas02-vol2-data --type f --owner yandy -X sudo chown yandy:users
fd . /mnt/nas03-data --type f --owner yandy -X sudo chown yandy:users
fd . /mnt/nas04-data --type f --owner yandy -X sudo chown yandy:users
fd . /mnt/docker --type f --owner yandy -X sudo chown yandy:users
fd . /mnt/docker-data --type f --owner yandy -X sudo chown yandy:users
```

- Change ownership of user only

```bash
fd . /mnt/nas01-data --type f --owner yandy -X sudo chown yandy
fd . /mnt/nas02-data --type f --owner yandy -X sudo chown yandy
fd . /mnt/nas02-vol2-data --type f --owner yandy -X sudo chown yandy
fd . /mnt/nas03-data --type f --owner yandy -X sudo chown yandy
fd . /mnt/nas04-data --type f --owner yandy -X sudo chown yandy
fd . /mnt/docker --type f --owner yandy -X sudo chown yandy
fd . /mnt/docker-data --type f --owner yandy -X sudo chown yandy
```

- Change ownership of group only

```bash
fd . /mnt/nas01-data --type f --owner yandy -X sudo chown :users
fd . /mnt/nas02-data --type f --owner yandy -X sudo chown :users
fd . /mnt/nas02-vol2-data --type f --owner yandy -X sudo chown :users
fd . /mnt/nas03-data --type f --owner yandy -X sudo chown :users
fd . /mnt/nas04-data --type f --owner yandy -X sudo chown :users
fd . /mnt/docker --type f --owner yandy -X sudo chown :users
fd . /mnt/docker-data --type f --owner yandy -X sudo chown :users
```

### Change Permissions

```bash
fd . /mnt/nas01-data --type f -X sudo chmod 664
fd . /mnt/nas02-data --type f -X sudo chmod 664
fd . /mnt/nas02-vol2-data --type f -X sudo chmod 664
fd . /mnt/nas03-data --type f -X sudo chmod 664
fd . /mnt/nas04-data --type f -X sudo chmod 664
fd . /mnt/nas01-data --type d -X sudo chmod 775
fd . /mnt/nas02-data --type d -X sudo chmod 775
fd . /mnt/nas02-vol2-data --type d -X sudo chmod 775
fd . /mnt/nas03-data --type d -X sudo chmod 775
fd . /mnt/nas04-data --type d -X sudo chmod 775
```

## Remove Some Files

```bash
sudo rm -rfv /mnt/data/usenet/anime/* ; \
sudo rm -rfv /mnt/data/usenet/audiobooks/* ; \
sudo rm -rfv /mnt/data/usenet/comics/* ; \
sudo rm -rfv /mnt/data/usenet/ebooks/* ; \
sudo rm -rfv /mnt/data/usenet/incomplete/* ; \
sudo rm -rfv /mnt/data/usenet/movies/* ; \
sudo rm -rfv /mnt/data/usenet/movies-4k/* ; \
sudo rm -rfv /mnt/data/usenet/music/* ; \
sudo rm -rfv /mnt/data/usenet/nzb/* ; \
sudo rm -rfv /mnt/data/usenet/prowlarr/* ; \
sudo rm -rfv /mnt/data/usenet/queue/* ; \
sudo rm -rfv /mnt/data/usenet/tmp/* ; \
sudo rm -rfv /mnt/data/usenet/tv/* ; \
sudo rm -rfv /mnt/data/usenet/tv-4k/*
```

## Remove Some Files 2

```bash
sudo rm -rfv /mnt/data/torrents/orphaned/* ; \
sudo rm -rfv /mnt/data/torrents2/orphaned/*
```
