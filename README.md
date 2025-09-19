# Porter - VM Disk Image Conversion & Upload Tool

Porter is a web application that simplifies the conversion of virtual machine disk images (VMDKs) to various formats and facilitates their upload to cloud storage providers.

![Porter Screenshot](https://via.placeholder.com/800x450?text=Porter+Screenshot)

## Features

- Extract OVA files to get VMDKs
- Convert VMDKs to multiple formats:
  - RAW (for Linux/KVM and AWS imports)
  - VHD (for Azure and older Hyper-V)
  - VHDX (for newer Hyper-V with better features)
  - QCOW2 (for QEMU and OpenStack)
- Upload converted images to:
  - AWS S3
  - Azure Blob Storage
  - Local filesystem
- Real-time progress tracking for uploads and extractions
- Clean, responsive web interface

## Quick Start

### Prerequisites

- Docker installed
- For cloud uploads:
  - AWS credentials in `~/.aws` (for AWS S3 uploads)
  - Azure CLI logged in (`~/.azure`) (for Azure Blob Storage uploads)

### Option 1: Using the Start Script

```bash
# Clone the repository
git clone https://github.com/MichaelCade/porter.git
cd porter

# Create data directories (if not using the script)
mkdir -p ~/porter-data/extracted ~/porter-data/converted

# Start Porter
./start.sh

# The web interface will automatically open at http://localhost:8080
```

### Option 2: Manual Docker Run

```bash
# Build the Docker image
docker build -t porter .

# Run the application with cloud provider credentials
docker run -d --name porter \
  -v ~/.aws:/root/.aws:ro \
  -v ~/.azure:/root/.azure:ro \
  -v ~/porter-data/extracted:/app/extracted \
  -v ~/porter-data/converted:/app/converted \
  -p 8080:8080 \
  porter
```

## Using the Web Interface

### 1. Extract OVA

- Click "Browse" in the Extract OVA section
- Select an OVA file from your computer
- Click "Extract" and wait for the process to complete
- The extracted VMDK files will appear in the Convert section

### 2. Convert VMDKs to Cloud Format

- Select the VMDKs you want to convert (all are selected by default)
- Choose the target format:
  - **RAW**: Most widely compatible format, but largest file size. Best for Linux/KVM and AWS imports.
  - **VHD**: Required for Azure and older Hyper-V environments.
  - **VHDX**: Enhanced VHD format for newer Hyper-V with larger disk size support and better performance.
  - **QCOW2**: Efficient format with compression and snapshot support. Best for QEMU/OpenStack.
- Click "Convert" and wait for the process to complete

### 3. Upload to Cloud

- Select the files you want to upload
- Choose your destination:
  - **Local**: Save to a local directory
  - **AWS S3**: Upload to an S3 bucket
  - **Azure Blob Storage**: Upload to Azure Blob Storage
- For cloud uploads, select the storage account and container/bucket
- Click "Upload" to start the transfer

## Data Storage

- Extracted VMDKs are stored in `~/porter-data/extracted`
- Converted files are stored in `~/porter-data/converted`

You can place VMDK files manually in the extraction directory if you want to skip the OVA extraction step.

## Stopping Porter

```bash
# Use the provided script
./stop.sh

# Or manually
docker stop porter
```

## Troubleshooting

- **Cloud credentials not found**: Ensure your AWS credentials are in `~/.aws` and Azure CLI is logged in (`~/.azure`)
- **Disk space issues**: Use `docker system df` to check Docker's disk usage. Run `docker system prune` to clear unused resources.
- **Permission problems**: Ensure the mounted volumes have appropriate permissions
- **Docker Desktop**: Ensure file sharing is enabled for the required directories
- **Conversion fails**: Check the Docker logs with `docker logs porter`

## Technical Details

Porter is built with:
- Go (backend)
- HTML/CSS/JavaScript (frontend)
- Docker (containerization)
- QEMU-utils (for disk conversion)

## License

MIT

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
