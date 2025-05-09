# Telegram Channel RSS Feed Generator

A Go application that generates RSS feeds from Telegram channels.

## Features

- Parse Telegram channel from Web pages.
- Cache fetched data in SQLite to minimize redundant network requests.
- Generate and serve RSS feeds in XML format.

## Installation

```sh
go install github.com/egor-lukin/tg-feeds@latest
```

## Usage

### Running the Application

To run the server, use:

```sh
./tg-feeds -dbpath /path/to/your/database.db -port 4567
```

### Parameters

- `-dbpath`: Path to the SQLite database file. Defaults to `./tg-feeds.db`.
- `-port`: Port on which the GIN server will run. Defaults to `4567`.

### Fetching RSS Feeds

To fetch the RSS feed for a specific Telegram channel, navigate to:

```sh
http://localhost:4567/<channel_name>
```

Replace `<channel_name>` with the name of the Telegram channel you want to get the RSS feed for.

### Ping Endpoint

To verify that the server is running, you can access the ping endpoint:

```sh
curl http://localhost:4567/ping
```

This should return a JSON response with the message "pong".

## Development 

### Clone and Setup

1. **Clone the repository:**

   ```sh
   git clone https://github.com/egor-lukin/tg-feeds.git
   cd tg-feeds
   ```

2. **Get the dependencies:**

   ```sh
   go mod download
   ```

3. **Build the application:**

   ```sh
   go build -o tg-feeds
   ```

### Creating a New Release

To trigger a new build and release:

1. **Create a new tag** (replace `v1.0.0` with your desired version):
   ```sh
   git tag v1.0.0
   git push origin v1.0.0
   ```

2. **GitHub Actions** will automatically build the binary and upload it to the [Releases](https://github.com/egor-lukin/tg-feeds/releases) page for the new tag.

## License

This project is licensed under the MIT License.
