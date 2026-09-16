$Zones = @{
  "plsnetlab.local" = @{
    MasterServers = @(
      "172.21.255.12",
      "172.21.25.13"
    )
  }
}


foreach ($Zone in $Zones.GetEnumerator()) {
  $ExistingZone = Get-DnsServerZone -Name $Zone.key -ErrorAction SilentlyContinue
  if ($null -eq $ExistingZone) {
    $ZoneFile = $DnsZone.key + ".dns"
    Add-DnsServerSecondaryZone -Name $Zone.key -ZoneFile $ZoneFile -MasterServers $Zone.Value.MasterServers
    Write-Host "Created zone" $Zone.key
  }
  else {
    Write-Host "Zone" $Zone.key "already exists, skipping"
  }
}


$DnsPtrZones = @{
  "10"  = @{
    NetworkId        = "10.0.0.0/8"
    ZoneName         = "10.in-addr.arpa"
    ReplicationScope = "Forest"
    MasterServers    = @(
      "172.21.255.12",
      "172.21.25.13"
    )
  }
  "172" = @{
    NetworkId        = "172.21.0.0/16"
    ZoneName         = "21.172.in-addr.arpa"
    ReplicationScope = "Forest"
    MasterServers    = @(
      "172.21.255.12",
      "172.21.25.13"
    )
  }
  "192" = @{
    NetworkId        = "192.168.0.0/16"
    ZoneName         = "168.192.in-addr.arpa"
    ReplicationScope = "Forest"
    MasterServers    = @(
      "172.21.255.12",
      "172.21.25.13"
    )
  }
}

# create reverse lookup zones if they don't exist
foreach ($DnsPtrZone in $DnsPtrZones.GetEnumerator()) {
  $ExistingPtrZone = Get-DnsServerZone -Name $DnsPtrZone.Value.ZoneName -ErrorAction SilentlyContinue
  if ($null -eq $ExistingPtrZone) {
    $ZoneFile = $DnsPtrZone.Value.ZoneName + ".dns"
    Add-DnsServerSecondaryZone -NetworkId $DnsPtrZone.Value.NetworkId -ZoneFile $ZoneFile -MasterServers $DnsPtrZone.Value.MasterServers
    Write-Host "Created zone $DnsPtrZone.Value.ZoneName"
  }
  else {
    Write-Host "Zone" $DnsPtrZone.Value.ZoneName "already exists, skipping"
  }
}