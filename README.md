<h1 align=center>Kevlar Server</h2>
<p align = center>Server for a real-time instant communication platform.</p>

# Description
This is a server for a complete real-time instant communication platform build entirely in Go. This server relies on MongoDB and MinIO Object Store for account data management and file storage respectively.

# Running
The server depends on MongoDB and MinIO to run. You can run them like this using podman.

MongoDB:
`
podman run -d -p 27017:27017 docker.io/library/mongo
`

MinIO:
`
podman run -p 9000:9000 quay.io/minio/minio server /data
`

Then, the server can run run like this:
`
go run main.go
`


# Features
* JSON based configuration support
* Websocket based real-time updates
* Account based file management
* Maximum account quota for storage
* Multimedia attachment support
* Support for setting profile images
* Automatic profile image resizing
* Support for setting arbitrary attributes on accounts

# Licence
 Copyright (C) 2024 Kartik Kukal

  This program is free software: you can redistribute it and/or modify
  it under the terms of the GNU Affero General Public License as
  published by the Free Software Foundation, either version 3 of the
  License, or (at your option) any later version.

  This program is distributed in the hope that it will be useful,
  but WITHOUT ANY WARRANTY; without even the implied warranty of
  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
  GNU Affero General Public License for more details.

  You should have received a copy of the GNU Affero General Public License
  along with this program.  If not, see <https://www.gnu.org/licenses/>.
