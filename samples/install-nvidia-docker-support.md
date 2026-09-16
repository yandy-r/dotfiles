## Install NVIDIA Driver

```bash
sudo apt update
sudo apt upgrade

sudo apt install ubuntu-drivers-common -y
sudo ubuntu-drivers devices

# Search apt for NVIDIA drivers.
sudo apt search nvidia-driver

# At the time of writing the latest driver is 550.

# Install the headless server drivers.
sudo apt install nvidia-headless-580-server -y
sudo apt install nvidia-utils-580-server -y

# Search apt for libnvidia-encode. Needed for Plex transcoding
sudo apt search nvidia-encode

# Install the libnvidia-encode package.
sudo apt install libnvidia-encode-580-server -y

sudo reboot
```

## Verify Installation

```bash
nvidia-smi
```

## Install NVIDIA Container Toolkit

```bash
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg \
 && curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
 sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
 sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list

sudo apt update; sudo apt install nvidia-docker2 -y
sudo apt install nvidia-container-toolkit -y

# Configure NVIDIA Container Toolkit
sudo nvidia-ctk runtime configure --runtime=docker

# Restart Docker
sudo systemctl restart docker

# Test GPU integration
docker run --gpus all nvidia/cuda:11.5.2-base-ubuntu20.04 nvidia-smi
```

## Install NVIDIA CUDA (OPTIONAL)

edit file /lib/udev/rules.d/40-vm-hotadd.rules >>

Comment this line out: SUBSYSTEM=="memory", ACTION=="add", PROGRAM="/bin/uname -p", RESULT!="s390\*", ATTR{state}=="offline", ATTR{state}="online"

wget <https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/x86_64/cuda-keyring_1.1-1_all.deb>
sudo dpkg -i cuda-keyring_1.1-1_all.deb
sudo apt update
sudo apt -y install cuda -y

sudo reboot

systemctl status nvidia-persistenced

export PATH=/usr/local/cuda/bin${PATH:+:${PATH}}
export LD_LIBRARY_PATH=/usr/local/cuda-12.2/lib64\
 ${LD_LIBRARY_PATH:+:${LD_LIBRARY_PATH}}

## Uninstall NVIDIA CUDA

sudo apt --purge remove "_cuda_" "_cublas_" "_cufft_" "_cufile_" "_curand_" \
 "_cusolver_" "_cusparse_" "_gds-tools_" "_npp_" "_nvjpeg_" "nsight\*" -y

sudo apt --purge remove "_nvidia_" -y

sudo apt autoremove -y
