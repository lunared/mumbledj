/*
 * MumbleDJ
 * By Matthieu Grieger
 * commands.go
 * Copyright (c) 2014 Matthieu Grieger (MIT License)
 */

package main

import (
	"errors"
	"fmt"
	"github.com/kennygrant/sanitize"
	"github.com/layeh/gumble/gumble"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Called on text message event. Checks the message for a command string, and processes it accordingly if
// it contains a command.
func parseCommand(user *gumble.User, username, command string) {
	var com, argument string
	sanitizedCommand := sanitize.HTML(command)
	if strings.Contains(sanitizedCommand, " ") {
		parsedCommand := strings.Split(sanitizedCommand, " ")
		com, argument = parsedCommand[0], parsedCommand[1]
	} else {
		com = command
		argument = ""
	}

	switch com {
	// Add command
	case dj.conf.Aliases.AddAlias:
		if dj.HasPermission(username, dj.conf.Permissions.AdminAdd) {
			add(user, username, argument)
		} else {
			user.Send(NO_PERMISSION_MSG)
		}
	// Skip command
	case dj.conf.Aliases.SkipAlias:
		if dj.HasPermission(username, dj.conf.Permissions.AdminSkip) {
			skip(user, username, false, false)
		} else {
			user.Send(NO_PERMISSION_MSG)
		}
	// Skip playlist command
	case dj.conf.Aliases.SkipPlaylistAlias:
		if dj.HasPermission(username, dj.conf.Permissions.AdminAddPlaylists) {
			skip(user, username, false, true)
		} else {
			user.Send(NO_PERMISSION_MSG)
		}
	// Forceskip command
	case dj.conf.Aliases.AdminSkipAlias:
		if dj.HasPermission(username, true) {
			skip(user, username, true, false)
		} else {
			user.Send(NO_PERMISSION_MSG)
		}
	// Playlist forceskip command
	case dj.conf.Aliases.AdminSkipPlaylistAlias:
		if dj.HasPermission(username, true) {
			skip(user, username, true, true)
		} else {
			user.Send(NO_PERMISSION_MSG)
		}
	// Volume command
	case dj.conf.Aliases.VolumeAlias:
		if dj.HasPermission(username, dj.conf.Permissions.AdminVolume) {
			volume(user, username, argument)
		} else {
			user.Send(NO_PERMISSION_MSG)
		}
	// Move command
	case dj.conf.Aliases.MoveAlias:
		if dj.HasPermission(username, dj.conf.Permissions.AdminMove) {
			move(user, argument)
		} else {
			user.Send(NO_PERMISSION_MSG)
		}
	// Reload command
	case dj.conf.Aliases.ReloadAlias:
		if dj.HasPermission(username, dj.conf.Permissions.AdminReload) {
			reload(user)
		} else {
			user.Send(NO_PERMISSION_MSG)
		}
	// Reset command
	case dj.conf.Aliases.ResetAlias:
		if dj.HasPermission(username, dj.conf.Permissions.AdminReset) {
			reset(username)
		} else {
			user.Send(NO_PERMISSION_MSG)
		}
	// Kill command
	case dj.conf.Aliases.KillAlias:
		if dj.HasPermission(username, dj.conf.Permissions.AdminKill) {
			kill()
		} else {
			user.Send(NO_PERMISSION_MSG)
		}
	default:
		user.Send(COMMAND_DOESNT_EXIST_MSG)
	}
}

// Performs add functionality. Checks input URL for YouTube format, and adds
// the URL to the queue if the format matches.
func add(user *gumble.User, username, url string) {
	if url == "" {
		user.Send(NO_ARGUMENT_MSG)
	} else {
		youtubePatterns := []string{
			`https?:\/\/www\.youtube\.com\/watch\?v=([\w-]+)`,
			`https?:\/\/youtube\.com\/watch\?v=([\w-]+)`,
			`https?:\/\/youtu.be\/([\w-]+)`,
			`https?:\/\/youtube.com\/v\/([\w-]+)`,
			`https?:\/\/www.youtube.com\/v\/([\w-]+)`,
		}
		matchFound := false
		shortUrl := ""

		for _, pattern := range youtubePatterns {
			if re, err := regexp.Compile(pattern); err == nil {
				if re.MatchString(url) {
					matchFound = true
					shortUrl = re.FindStringSubmatch(url)[1]
					break
				}
			}
		}

		if matchFound {
			newSong := NewSong(username, shortUrl)
			if err := dj.queue.AddItem(newSong); err == nil {
				dj.client.Self().Channel().Send(fmt.Sprintf(SONG_ADDED_HTML, username, newSong.title), false)
				if dj.queue.Len() == 1 && !dj.audioStream.IsPlaying() {
					if err := dj.queue.CurrentItem().(*Song).Download(); err == nil {
						dj.queue.CurrentItem().(*Song).Play()
					} else {
						user.Send(AUDIO_FAIL_MSG)
						dj.queue.CurrentItem().(*Song).Delete()
					}
				}
			}
		} else {
			// Check to see if we have a playlist URL instead.
			youtubePlaylistPattern := `https?:\/\/www\.youtube\.com\/playlist\?list=(\w+)`
			if re, err := regexp.Compile(youtubePlaylistPattern); err == nil {
				if re.MatchString(url) {
					if dj.HasPermission(username, dj.conf.Permissions.AdminAddPlaylists) {
						shortUrl = re.FindStringSubmatch(url)[1]
						newPlaylist := NewPlaylist(username, shortUrl)
						if dj.queue.AddItem(newPlaylist); err == nil {
							dj.client.Self().Channel().Send(fmt.Sprintf(PLAYLIST_ADDED_HTML, username, newPlaylist.title), false)
							if dj.queue.Len() == 1 && !dj.audioStream.IsPlaying() {
								if err := dj.queue.CurrentItem().(*Playlist).songs.CurrentItem().(*Song).Download(); err == nil {
									dj.queue.CurrentItem().(*Playlist).songs.CurrentItem().(*Song).Play()
								} else {
									user.Send(AUDIO_FAIL_MSG)
									dj.queue.CurrentItem().(*Playlist).songs.CurrentItem().(*Song).Delete()
								}
							}
						}
					} else {
						user.Send(NO_PLAYLIST_PERMISSION_MSG)
					}
				} else {
					user.Send(INVALID_URL_MSG)
				}
			}
		}
	}
}

// Performs skip functionality. Adds a skip to the skippers slice for the current song, and then
// evaluates if a skip should be performed. Both skip and forceskip are implemented here.
func skip(user *gumble.User, username string, admin, playlistSkip bool) {
	if dj.audioStream.IsPlaying() {
		if playlistSkip {
			if dj.queue.CurrentItem().ItemType() == "playlist" {
				if err := dj.queue.CurrentItem().AddSkip(username); err == nil {
					if admin {
						dj.client.Self().Channel().Send(ADMIN_PLAYLIST_SKIP_MSG, false)
					} else {
						dj.client.Self().Channel().Send(fmt.Sprintf(PLAYLIST_SKIP_ADDED_HTML, username), false)
					}
					if dj.queue.CurrentItem().SkipReached(len(dj.client.Self().Channel().Users())) || admin {
						dj.queue.CurrentItem().(*Playlist).skipped = true
						dj.client.Self().Channel().Send(PLAYLIST_SKIPPED_HTML, false)
						if err := dj.audioStream.Stop(); err != nil {
							panic(errors.New("An error occurred while stopping the current song."))
						}
					}
				}
			} else {
				user.Send(NO_PLAYLIST_PLAYING_MSG)
			}
		} else {
			var currentItem QueueItem
			if dj.queue.CurrentItem().ItemType() == "playlist" {
				currentItem = dj.queue.CurrentItem().(*Playlist).songs.CurrentItem()
			} else {
				currentItem = dj.queue.CurrentItem()
			}
			if err := currentItem.AddSkip(username); err == nil {
				if admin {
					dj.client.Self().Channel().Send(ADMIN_SONG_SKIP_MSG, false)
				} else {
					dj.client.Self().Channel().Send(fmt.Sprintf(SKIP_ADDED_HTML, username), false)
				}
				if currentItem.SkipReached(len(dj.client.Self().Channel().Users())) || admin {
					dj.client.Self().Channel().Send(SONG_SKIPPED_HTML, false)
					if err := dj.audioStream.Stop(); err != nil {
						panic(errors.New("An error occurred while stopping the current song."))
					}
				}
			}
		}
	} else {
		user.Send(NO_MUSIC_PLAYING_MSG)
	}
}

// Performs volume functionality. Checks input value against LowestVolume and HighestVolume from
// config to determine if the volume should be applied. If in the correct range, the new volume
// is applied and is immediately in effect.
func volume(user *gumble.User, username, value string) {
	if value == "" {
		dj.client.Self().Channel().Send(fmt.Sprintf(CUR_VOLUME_HTML, dj.audioStream.Volume()), false)
	} else {
		if parsedVolume, err := strconv.ParseFloat(value, 32); err == nil {
			newVolume := float32(parsedVolume)
			if newVolume >= dj.conf.Volume.LowestVolume && newVolume <= dj.conf.Volume.HighestVolume {
				dj.audioStream.SetVolume(newVolume)
				dj.client.Self().Channel().Send(fmt.Sprintf(VOLUME_SUCCESS_HTML, username, dj.audioStream.Volume()), false)
			} else {
				user.Send(fmt.Sprintf(NOT_IN_VOLUME_RANGE_MSG, dj.conf.Volume.LowestVolume, dj.conf.Volume.HighestVolume))
			}
		} else {
			user.Send(fmt.Sprintf(NOT_IN_VOLUME_RANGE_MSG, dj.conf.Volume.LowestVolume, dj.conf.Volume.HighestVolume))
		}
	}
}

// Performs move functionality. Determines if the supplied channel is valid and moves the bot
// to the channel if it is.
func move(user *gumble.User, channel string) {
	if channel == "" {
		user.Send(NO_ARGUMENT_MSG)
	} else {
		if dj.client.Channels().Find(channel) != nil {
			dj.client.Self().Move(dj.client.Channels().Find(channel))
		} else {
			user.Send(CHANNEL_DOES_NOT_EXIST_MSG)
		}
	}
}

// Performs reload functionality. Tells command submitter if the reload completed successfully.
func reload(user *gumble.User) {
	if err := loadConfiguration(); err == nil {
		user.Send(CONFIG_RELOAD_SUCCESS_MSG)
	} else {
		panic(err)
	}
}

// Performs reset functionality. Clears the song queue, stops playing audio, and deletes all
// remaining songs in the ~/.mumbledj/songs directory.
func reset(username string) {
	dj.queue.queue = dj.queue.queue[:0]
	if err := dj.audioStream.Stop(); err == nil {
		if err := deleteSongs(); err == nil {
			dj.client.Self().Channel().Send(fmt.Sprintf(QUEUE_RESET_HTML, username), false)
		} else {
			panic(err)
		}
	} else {
		panic(err)
	}
}

// Performs kill functionality. First cleans the ~/.mumbledj/songs directory to get rid of any
// excess m4a files. The bot then safely disconnects from the server.
func kill() {
	if err := deleteSongs(); err != nil {
		panic(err)
	}
	if err := dj.client.Disconnect(); err == nil {
		fmt.Println("Kill successful. Goodbye!")
		os.Exit(0)
	} else {
		panic(errors.New("An error occurred while disconnecting from the server."))
	}
}

// Deletes songs from ~/.mumbledj/songs.
func deleteSongs() error {
	songsDir := fmt.Sprintf("%s/.mumbledj/songs", dj.homeDir)
	if err := os.RemoveAll(songsDir); err != nil {
		return errors.New("An error occurred while deleting the audio files.")
	} else {
		if err := os.Mkdir(songsDir, 0777); err != nil {
			return errors.New("An error occurred while recreating the songs directory.")
		}
		return nil
	}
	return nil
}
