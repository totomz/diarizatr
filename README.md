# diarizatr

A simple Golang executable that get a path to an audio file
created by OBS Studio and convert it to diarized text using AssemblyAI.

The output is a plain text file, without timings, ready to be fed to an LLM.

## Install & Setup
1. Build and install
```
./shMakefile install
```

2. Export your AssemblyAI API key:
```
export Z_ASSEMBLYAI_KEY=<your key>
```

3. OBS setup

In Settings → Output → Recording → Recording Format select **MP4** 

## Usage

Transcription is asynchronous and happens in two steps.
```
# Upload the file
diarizatr -lang it /path/to/recording.mp4

# save the transcription in the current dir
diarizatr -id <id>
```
# Note
AssemblyAI automatically deletes uploaded audio within 24–48 hours.

Daje
