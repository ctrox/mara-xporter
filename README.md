# mara-xporter

This is a Prometheus exporter for the Coffee Machine Lelit Mara PL62X. It uses
the UART interface to get some basic readings of the heat exchanger, steam
temperature etc.

## raspberry-pi config

* build binary (1st gen Pi Zero)

	```bash
	env GOOS=linux GOARCH=arm GOARM=5 go build
	```

* enable UART in pi config

	```bash
	$ vi /boot/config.txt
	enable_uart=1
	```

* copy binary and enable systemd unit

	```bash
	cp mara-xporter /usr/local/bin
	cp mara-xporter.service /etc/systemd/system/
	systemctl daemon-reload
	systemctl enable mara-xporter
	```
