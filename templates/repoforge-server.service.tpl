[Unit]
Description=RepoForge Offline Repository HTTP Server
After=network.target

[Service]
Type=simple
Environment=REPOFORGE_HOME={{ .Home }}
ExecStart={{ .Executable }} server start
Restart={{ .Restart }}
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true
ReadOnlyPaths={{ .RepoDir }}

[Install]
WantedBy=multi-user.target
