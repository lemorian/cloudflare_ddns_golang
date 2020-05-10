The code is set to run every 5 mins, check if the ip has changed, if so, it will update the DNS record in Cloudflare server.

Input the following details in config.json

{
    "authEmail": "",
    "authKey": "",
    "zoneIdentifier": "",
    "recordName": "",
    "proxy": true
}

You can find more details on generating AuthKey here.
https://api.cloudflare.com/#getting-started-endpoints
