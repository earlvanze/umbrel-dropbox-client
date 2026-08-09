#!/usr/bin/env python3
# Nautilus extension placeholder. Install to ~/.local/share/nautilus-python/extensions/.
# Provides context menu hooks that call `umbrel-dropbox-client`.
from gi.repository import Nautilus, GObject
import subprocess

class UmbrelDropboxClientExtension(GObject.GObject, Nautilus.MenuProvider):
    def get_file_items(self, files):
        if not files: return []
        item = Nautilus.MenuItem(name='UmbrelDropboxClient::SyncNow', label='SyncNest: Sync now', tip='Queue this path for sync')
        item.connect('activate', self.sync_now, files)
        return [item]
    def sync_now(self, menu, files):
        for f in files:
            path = f.get_location().get_path()
            subprocess.Popen(['umbrel-dropbox-client', 'sync', '--once', '--path', path], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
