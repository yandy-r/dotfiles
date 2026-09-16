#!/bin/bash

# List the network services on the system.
sudo networksetup -listallnetworkservices

# This script adds routes to the routing table for the specified interface.
sudo networksetup -setadditionalroutes "Wi-Fi" 10.0.0.0 255.0.0.0 10.25.0.1 192.168.0.0 255.255.0.0 10.25.0.1 172.16.0.0 255.240.0.0 10.25.0.1
sudo networksetup -setadditionalroutes "USB 10G LAN" 10.0.0.0 255.0.0.0 10.25.0.1 192.168.0.0 255.255.0.0 10.25.0.1 172.16.0.0 255.240.0.0 10.25.0.1
sudo networksetup -setadditionalroutes "USB 2.5G LAN" 10.0.0.0 255.0.0.0 10.25.0.1 192.168.0.0 255.255.0.0 10.25.0.1 172.16.0.0 255.240.0.0 10.25.0.1
sudo networksetup -setadditionalroutes "USB 2.5G LAN-2" 10.0.0.0 255.0.0.0 10.25.0.1 192.168.0.0 255.255.0.0 10.25.0.1 172.16.0.0 255.240.0.0 10.25.0.1
sudo networksetup -setadditionalroutes "USB 2.5G LAN-3" 10.0.0.0 255.0.0.0 10.25.0.1 192.168.0.0 255.255.0.0 10.25.0.1 172.16.0.0 255.240.0.0 10.25.0.1
sudo networksetup -setadditionalroutes "USB 1G LAN" 10.0.0.0 255.0.0.0 10.25.0.1 192.168.0.0 255.255.0.0 10.25.0.1 172.16.0.0 255.240.0.0 10.25.0.1

# To: Remove routes from the routing table for the specified interface.
sudo networksetup -setadditionalroutes "Wi-Fi"
sudo networksetup -setadditionalroutes "USB 10G LAN"
sudo networksetup -setadditionalroutes "USB 2.5G LAN"
sudo networksetup -setadditionalroutes "USB 2.5G LAN-2"
sudo networksetup -setadditionalroutes "USB 2.5G LAN-3"
sudo networksetup -setadditionalroutes "USB 1G LAN"
