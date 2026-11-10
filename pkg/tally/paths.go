package tally

/*
DefaultCeremonyDir is the standard location for keyholder shares and
ceremony metadata inside the tally-service container. The directory is
mounted from deployments/fabric/ceremony/ on the host.
*/
const DefaultCeremonyDir = "/ceremony"

/*
DefaultPassphrasePrefix is the passphrase prefix used by the dev key
ceremony. Each keyholder N's share is encrypted with prefix + N (e.g.
"dev-keyholder-1"). Production replaces this with keyholder-provided
passphrases.
*/
const DefaultPassphrasePrefix = "dev-keyholder-"
