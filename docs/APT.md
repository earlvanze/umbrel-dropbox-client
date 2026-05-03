# APT repository plan

The release pipeline builds `.deb` packages with GoReleaser/nfpm.

Initial install path:

```bash
curl -fsSL https://raw.githubusercontent.com/earlvanze/umbrel-dropbox-client/master/scripts/install-linux.sh | bash
```

APT source target after first signed repo publish:

```bash
curl -fsSL https://earlvanze.github.io/umbrel-dropbox-client/apt/key.gpg | sudo gpg --dearmor -o /usr/share/keyrings/umbrel-dropbox-client.gpg
echo "deb [signed-by=/usr/share/keyrings/umbrel-dropbox-client.gpg] https://earlvanze.github.io/umbrel-dropbox-client/apt stable main" | sudo tee /etc/apt/sources.list.d/umbrel-dropbox-client.list
sudo apt update
sudo apt install umbrel-dropbox-client
```

The current `scripts/add-apt-source.sh` intentionally does not mutate `/etc/apt` until the signed repo exists.
