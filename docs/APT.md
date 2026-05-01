# APT repository plan

The release pipeline builds `.deb` packages with GoReleaser/nfpm.

Initial install path:

```bash
curl -fsSL https://raw.githubusercontent.com/earlvanze/umbrel-dropbox-sync/master/scripts/install-linux.sh | bash
```

APT source target after first signed repo publish:

```bash
curl -fsSL https://earlvanze.github.io/umbrel-dropbox-sync/apt/key.gpg | sudo gpg --dearmor -o /usr/share/keyrings/umbrel-dropbox-sync.gpg
echo "deb [signed-by=/usr/share/keyrings/umbrel-dropbox-sync.gpg] https://earlvanze.github.io/umbrel-dropbox-sync/apt stable main" | sudo tee /etc/apt/sources.list.d/umbrel-dropbox-sync.list
sudo apt update
sudo apt install umbrel-dropbox-sync
```

The current `scripts/add-apt-source.sh` intentionally does not mutate `/etc/apt` until the signed repo exists.
