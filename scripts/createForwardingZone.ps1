$Zones = @{
  "plsnetlab.local" = @{
    MasterServers = @(
      "172.21.255.12",
      "172.21.255.13"
    )
  }
  "plsnetlab.com"     = @{
    MasterServers = @(
      "172.21.255.12",
      "172.21.255.13"
    )
  }
}

foreach ($Zone in $Zones.GetEnumerator()) {
  $ZoneName = $Zone.Key
  $MasterServers = $Zone.Value.MasterServers

  Write-Host $MasterServers
  # check if zone exists, if it does, skip
  if (Get-DnsServerZone -Name $ZoneName -ErrorAction SilentlyContinue) {
    Write-Host "Zone $ZoneName already exists, skipping"
    continue
  }

  # create zone
  Add-DnsServerConditionalForwarderZone -MasterServers $MasterServers -Name $ZoneName
  Write-Host "Created zone $ZoneName"
}