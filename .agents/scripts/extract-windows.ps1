param([string]$Archive, [string]$Destination)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip = [System.IO.Compression.ZipFile]::OpenRead($Archive)
try {
    if ($zip.Entries.Count -ne 1 -or $zip.Entries[0].FullName -ne 'grepple.exe') {
        throw 'Unexpected Grepple release archive contents'
    }
    [System.IO.Compression.ZipFileExtensions]::ExtractToFile($zip.Entries[0], $Destination, $false)
} finally {
    $zip.Dispose()
}
