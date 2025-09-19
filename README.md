# Porter - VM Disk Image Conversion & Upload Tool

Porter is a simple web app that helps convert VM disk images (VMDK) to different formats and upload them to various cloud storage providers.

## Features

- Extract OVA files to get VMDKs
- Convert VMDKs to RAW or VHD formats
- Upload converted images to:
  - AWS S3
  - Azure Blob Storage
  - Local filesystem

## Usage with Docker

### Prerequisites

- Docker installed
- For cloud uploads:
  - AWS credentials in `~/.aws` (for AWS S3 uploads)
  - Azure CLI logged in (`~/.azure`) (for Azure Blob Storage uploads)

### Quick Start

```bash
# Use the provided script to start the application
./run.sh

# Access the web interface at http://localhost:8080
```

### Running in Background

```bash
# Use the start script to run in the background and open browser
./start.sh
```

### Manual Docker Run

```bash
# Build the Docker image
docker build -t porter .

# Run the application with cloud provider credentials
docker run -it --rm \
  -v ~/.aws:/root/.aws \
  -v ~/.azure:/root/.azure \
  -v ~/porter-data/extracted:/app/extracted \
  -v ~/porter-data/converted:/app/converted \
  -p 8080:8080 \
  porter
```

## Using the Web Interface

1. **Extract OVA**: Upload an OVA file to extract the VMDK files inside
2. **Convert VMDKs**: Select the extracted VMDKs and choose your target format (RAW or VHD)
3. **Upload**: Select the converted files and choose your destination (AWS S3, Azure Blob Storage, or local folder)

## Data Storage

- Extracted VMDKs are stored in `~/porter-data/extracted`
- Converted files are stored in `~/porter-data/converted`

You can place VMDK files manually in the extraction directory if you want to skip the OVA extraction step.

## Troubleshooting

- If you don't see your AWS buckets or Azure containers, ensure your credentials are properly configured
- Check that the mounted volumes have appropriate permissions
- If using Docker Desktop, ensure file sharing is enabled for the required directories
