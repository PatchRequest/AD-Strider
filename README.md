# AD Strider
A security automation tool to detect misconfigurations in an active directory by analyzing data from [Bloodhound](https://github.com/BloodHoundAD/BloodHound)

# Configuration
The config file "config.json" contains a  two list of all connections between AD-Objects. Each list is for a specific direction (T1 -> T0 or T0 -> T1)
If the value is set to true the connection will be marked as misconfiguration

Example: 

__A User from Tier1 should not have admin permissions for a Tier0 object__
```
[...]
"IntoT0": {
        "AdminTo": true,
[...]
```
__A User from Tier0 is allowed to have admin permissions for a Tier1 object__
```
[...]
"IntoT1": {
        "AdminTo": false,
[...]
```
# Setup
Download the latest release for your platform from Github [here](https://github.com/PatchRequest/AD-Strider/releases)
or build it on your own with
```
go build .
```

# Usage


# Licence
