# HillviewTV Video API Documentation

## Overview

The HillviewTV Video API provides a comprehensive set of endpoints for managing videos, playlists, spotlights, newsletter subscriptions, and video uploads. The API is built using Go with Gorilla Mux for routing and integrates with Cloudflare Stream for video hosting.

**Base URL:** `/video/v1.1`

**Port:** Configured via environment variable

---

## Table of Contents

- [Authentication](#authentication)
- [Response Format](#response-format)
- [Error Handling](#error-handling)
- [Endpoints](#endpoints)
  - [Health Check](#health-check)
  - [Videos](#videos)
  - [Playlists](#playlists)
  - [Spotlights](#spotlights)
  - [Newsletter](#newsletter)
  - [Upload](#upload)
  - [Cloudflare Integration](#cloudflare-integration)

---

## Authentication

Some endpoints require authentication via an access token. Protected endpoints are marked with 🔒.

**Authorization Header:**

```
Authorization: Bearer <access_token>
```

---

## Response Format

### Success Response

All successful responses follow this structure:

```json
{
  "success": true,
  "message": "request was successful",
  "data": { ... }
}
```

### Error Response

Error responses follow this structure:

```json
{
  "error": "error details or null",
  "error_message": "human readable error message",
  "error_code": 1000
}
```

---

## Error Handling

| HTTP Status | Description                                      |
| ----------- | ------------------------------------------------ |
| `200`       | Success                                          |
| `201`       | Created                                          |
| `204`       | No Content (Success with no response body)       |
| `400`       | Bad Request - Missing or invalid parameters      |
| `401`       | Unauthorized - Missing or invalid authentication |
| `404`       | Not Found - Resource does not exist              |
| `409`       | Conflict - Resource already exists               |
| `413`       | Request Entity Too Large - File too large        |
| `500`       | Internal Server Error                            |
| `501`       | Not Implemented                                  |

---

## Endpoints

---

### Health Check

#### `GET /healthcheck`

Check if the API service is running and healthy.

**Response:**

- `200 OK` - Service is healthy

---

## Videos

### List Videos

#### `GET /video/v1.1/list/videos`

Retrieve a paginated list of videos.

**Query Parameters:**

| Parameter | Type    | Required | Default | Description                        |
| --------- | ------- | -------- | ------- | ---------------------------------- |
| `limit`   | integer | ✅       | -       | Maximum number of videos to return |
| `offset`  | integer | ✅       | -       | Number of videos to skip           |
| `sort`    | string  | ❌       | `desc`  | Sort order (`asc` or `desc`)       |
| `by`      | string  | ❌       | `date`  | Sort by field (`date` or `views`)  |
| `search`  | string  | ❌       | -       | Search query to filter videos      |

**Example Request:**

```bash
curl -X GET "https://api.example.com/video/v1.1/list/videos?limit=10&offset=0&sort=desc&by=date"
```

**Success Response (200):**

```json
{
  "success": true,
  "message": "Videos retrieved successfully",
  "data": [
    {
      "id": 1,
      "uuid": "abc123-def456",
      "title": "Video Title",
      "description": "Video description",
      "thumbnail": "https://content.hillview.tv/thumbnails/image.jpg",
      "cloudflare_id": "cf_id_123",
      "url": "https://videodelivery.net/cf_id_123/manifest/video.m3u8",
      "download_url": "https://download.url/video.mp4",
      "allow_downloads": true,
      "views": 1234,
      "status": {
        "id": 1,
        "name": "Active",
        "short_name": "active"
      },
      "inserted_at": "2024-01-15T12:00:00Z"
    }
  ]
}
```

---

### Get Video by ID

#### `GET /video/v1.1/read/videoByID/{id}`

Retrieve a specific video by its numeric ID.

**Path Parameters:**

| Parameter | Type    | Required | Description            |
| --------- | ------- | -------- | ---------------------- |
| `id`      | integer | ✅       | The video's numeric ID |

**Example Request:**

```bash
curl -X GET "https://api.example.com/video/v1.1/read/videoByID/123"
```

**Success Response (200):**

```json
{
  "success": true,
  "message": "Video retrieved successfully",
  "data": {
    "id": 123,
    "uuid": "abc123-def456",
    "title": "Video Title",
    "description": "Video description",
    "thumbnail": "https://content.hillview.tv/thumbnails/image.jpg",
    "cloudflare_id": "cf_id_123",
    "url": "https://videodelivery.net/cf_id_123/manifest/video.m3u8",
    "download_url": "https://download.url/video.mp4",
    "allow_downloads": true,
    "views": 1234,
    "status": {
      "id": 1,
      "name": "Active",
      "short_name": "active"
    },
    "inserted_at": "2024-01-15T12:00:00Z"
  }
}
```

---

### Get Video (v2.1)

#### `GET /video/v1.1/video/{query}`

Retrieve a video by either its numeric ID or UUID.

**Path Parameters:**

| Parameter | Type           | Required | Description                               |
| --------- | -------------- | -------- | ----------------------------------------- |
| `query`   | string/integer | ✅       | The video's ID (integer) or UUID (string) |

**Example Requests:**

```bash
# By ID
curl -X GET "https://api.example.com/video/v1.1/video/123"

# By UUID
curl -X GET "https://api.example.com/video/v1.1/video/abc123-def456"
```

**Success Response (200):**

```json
{
  "success": true,
  "message": "Video retrieved successfully",
  "data": {
    "id": 123,
    "uuid": "abc123-def456",
    "title": "Video Title",
    "description": "Video description",
    "thumbnail": "https://content.hillview.tv/thumbnails/image.jpg",
    "cloudflare_id": "cf_id_123",
    "url": "https://videodelivery.net/cf_id_123/manifest/video.m3u8",
    "download_url": "https://download.url/video.mp4",
    "allow_downloads": true,
    "views": 1234,
    "status": {
      "id": 1,
      "name": "Active",
      "short_name": "active"
    },
    "inserted_at": "2024-01-15T12:00:00Z"
  }
}
```

**Error Response (404):**

```json
{
  "error": null,
  "error_message": "video not found",
  "error_code": 1000
}
```

---

### Create Video

#### `POST /video/v1.1/create/video`

Create a new video entry in the database.

**Request Body:**

| Field         | Type   | Required | Description             |
| ------------- | ------ | -------- | ----------------------- |
| `title`       | string | ✅       | The video title         |
| `url`         | string | ✅       | The video stream URL    |
| `description` | string | ✅       | The video description   |
| `thumbnail`   | string | ❌       | The thumbnail image URL |

**Example Request:**

```bash
curl -X POST "https://api.example.com/video/v1.1/create/video" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "My New Video",
    "url": "https://videodelivery.net/abc123/manifest/video.m3u8",
    "description": "This is a great video",
    "thumbnail": "https://content.hillview.tv/thumbnails/image.jpg"
  }'
```

**Success Response (200):**

```json
{
  "success": true,
  "message": "request was successful",
  "data": {
    "id": 456,
    "uuid": "new-uuid-here"
  }
}
```

---

### Record View

#### `POST /video/v1.1/recordView/{query}`

Record a view for a video. Increments the view counter.

**Path Parameters:**

| Parameter | Type           | Required | Description                               |
| --------- | -------------- | -------- | ----------------------------------------- |
| `query`   | string/integer | ✅       | The video's ID (integer) or UUID (string) |

**Example Request:**

```bash
curl -X POST "https://api.example.com/video/v1.1/recordView/123"
```

**Responses:**

- `204 No Content` - View recorded successfully
- `400 Bad Request` - Missing query parameter
- `404 Not Found` - Video not found
- `500 Internal Server Error` - Server error

---

### Record Download

#### `POST /video/v1.1/recordDownload/{query}`

Record a download for a video. Increments the download counter.

**Path Parameters:**

| Parameter | Type           | Required | Description                               |
| --------- | -------------- | -------- | ----------------------------------------- |
| `query`   | string/integer | ✅       | The video's ID (integer) or UUID (string) |

**Example Request:**

```bash
curl -X POST "https://api.example.com/video/v1.1/recordDownload/123"
```

**Responses:**

- `204 No Content` - Download recorded successfully
- `400 Bad Request` - Missing query parameter
- `404 Not Found` - Video not found
- `500 Internal Server Error` - Server error

---

## Playlists

### List Playlists

#### `GET /video/v1.1/list/playlists`

Retrieve a paginated list of playlists.

**Query Parameters:**

| Parameter | Type    | Required | Default | Description                           |
| --------- | ------- | -------- | ------- | ------------------------------------- |
| `limit`   | integer | ✅       | -       | Maximum number of playlists to return |
| `offset`  | integer | ✅       | -       | Number of playlists to skip           |
| `sort`    | string  | ❌       | `desc`  | Sort order (`asc` or `desc`)          |

**Example Request:**

```bash
curl -X GET "https://api.example.com/video/v1.1/list/playlists?limit=10&offset=0&sort=desc"
```

**Success Response (200):**

```json
{
  "success": true,
  "message": "Playlists retrieved successfully",
  "data": [
    {
      "id": 1,
      "name": "Playlist Name",
      "description": "Playlist description",
      "banner_image": "https://content.hillview.tv/banners/image.jpg",
      "route": "playlist-slug",
      "inserted_at": "2024-01-15T12:00:00Z",
      "videos": [
        {
          "id": 1,
          "uuid": "abc123",
          "title": "Video Title",
          "..."
        }
      ]
    }
  ]
}
```

---

### Get Playlist

#### `GET /video/v1.1/read/playlist`

Retrieve a specific playlist by ID or route.

**Query Parameters:**

| Parameter | Type    | Required | Description                   |
| --------- | ------- | -------- | ----------------------------- |
| `id`      | integer | ❌\*     | The playlist's numeric ID     |
| `route`   | string  | ❌\*     | The playlist's URL route/slug |

\*At least one of `id` or `route` must be provided.

**Example Requests:**

```bash
# By ID
curl -X GET "https://api.example.com/video/v1.1/read/playlist?id=1"

# By Route
curl -X GET "https://api.example.com/video/v1.1/read/playlist?route=drama-2024"
```

**Success Response (200):**

```json
{
  "success": true,
  "message": "Playlist retrieved successfully",
  "data": {
    "id": 1,
    "name": "Drama 2024",
    "description": "All drama productions from 2024",
    "banner_image": "https://content.hillview.tv/banners/drama2024.jpg",
    "route": "drama-2024",
    "inserted_at": "2024-01-15T12:00:00Z",
    "videos": [...]
  }
}
```

---

## Spotlights

### List Spotlights

#### `GET /video/v1.1/spotlight`

Retrieve a paginated list of spotlight videos (featured content).

**Query Parameters:**

| Parameter | Type    | Required | Description                            |
| --------- | ------- | -------- | -------------------------------------- |
| `limit`   | integer | ✅       | Maximum number of spotlights to return |
| `offset`  | integer | ✅       | Number of spotlights to skip           |

**Example Request:**

```bash
curl -X GET "https://api.example.com/video/v1.1/spotlight?limit=5&offset=0"
```

**Success Response (200):**

```json
{
  "success": true,
  "message": "request was successful",
  "data": [
    {
      "rank": 1,
      "video_id": 123,
      "inserted_at": "2024-01-15T12:00:00Z",
      "updated_at": "2024-01-16T12:00:00Z",
      "video": {
        "id": 123,
        "uuid": "abc123",
        "title": "Featured Video",
        "description": "This is a featured video",
        "thumbnail": "https://content.hillview.tv/thumbnails/featured.jpg",
        "..."
      }
    }
  ]
}
```

---

## Newsletter

### Subscribe to Newsletter

#### `POST /video/v1.1/newsletter`

Subscribe an email address to the HillviewTV newsletter.

**Request Body:**

| Field   | Type   | Required | Description           |
| ------- | ------ | -------- | --------------------- |
| `email` | string | ✅       | A valid email address |

**Example Request:**

```bash
curl -X POST "https://api.example.com/video/v1.1/newsletter" \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com"}'
```

**Responses:**

- `204 No Content` - Successfully subscribed
- `400 Bad Request` - Invalid or missing email
- `409 Conflict` - Email already subscribed

**Notes:**

- A confirmation email will be sent to the subscriber via SendGrid
- Invalid email formats will be rejected

---

### Unsubscribe from Newsletter

#### `POST /video/v1.1/newsletter/unsubscribe`

Unsubscribe an email address from the HillviewTV newsletter.

**Request Body:**

| Field   | Type   | Required | Description                      |
| ------- | ------ | -------- | -------------------------------- |
| `email` | string | ✅       | The email address to unsubscribe |

**Example Request:**

```bash
curl -X POST "https://api.example.com/video/v1.1/newsletter/unsubscribe" \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com"}'
```

**Responses:**

- `201 Created` - Successfully unsubscribed
- `400 Bad Request` - Missing email
- `409 Conflict` - Email not found or already unsubscribed
- `500 Internal Server Error` - Failed to unsubscribe from SendGrid

---

## Upload

### Upload Video 🔒

#### `POST /video/v1.1/upload/video`

Upload a video file to S3 and process it through Cloudflare Stream.

**Authentication:** Required (Access Token)

**Request:**

- Content-Type: `multipart/form-data`
- Form field: `upload` - The video file (MP4)

**Example Request:**

```bash
curl -X POST "https://api.example.com/video/v1.1/upload/video" \
  -H "Authorization: Bearer <access_token>" \
  -F "upload=@/path/to/video.mp4"
```

**Success Response (200):**

```json
{
  "success": true,
  "message": "request was successful",
  "data": {
    "url": "https://videodelivery.net/cf_uid/manifest/video.m3u8",
    "thumbnail": "https://videodelivery.net/cf_uid/thumbnails/thumbnail.jpg",
    "s3_url": "https://content.hillview.tv/videos/uploads/UID1-1234567890-ABCDEFGHIJ.mp4"
  }
}
```

**Notes:**

- Video is first uploaded to S3, then copied to Cloudflare Stream
- The generated filename includes the user ID, timestamp, and random string
- Temporary files are automatically cleaned up after upload

---

### Upload Thumbnail 🔒

#### `POST /video/v1.1/upload/thumbnail`

Upload a thumbnail image to S3.

**Authentication:** Required (Access Token)

**Request:**

- Content-Type: `multipart/form-data`
- Form field: `upload` - The image file (JPEG)

**Example Request:**

```bash
curl -X POST "https://api.example.com/video/v1.1/upload/thumbnail" \
  -H "Authorization: Bearer <access_token>" \
  -F "upload=@/path/to/thumbnail.jpg"
```

**Success Response (200):**

```json
{
  "success": true,
  "message": "Thumbnail uploaded successfully",
  "data": {
    "url": "https://content.hillview.tv/thumbnails/UID1-1234567890-ABCDEFGHIJ.jpg"
  }
}
```

---

## Cloudflare Integration

### Cloudflare Direct Upload 🔒

#### `POST /video/v1.1/upload/cf/upload`

Initiate a direct upload to Cloudflare Stream using the TUS protocol.

**Authentication:** Required (Access Token)

**Request Headers:**

| Header            | Required | Description                |
| ----------------- | -------- | -------------------------- |
| `Tus-Resumable`   | ✅       | Must be `1.0.0`            |
| `Upload-Length`   | ✅       | Total upload size in bytes |
| `Upload-Metadata` | ✅       | Base64-encoded metadata    |
| `Origin`          | ✅       | Request origin             |

**Example Request:**

```bash
curl -X POST "https://api.example.com/video/v1.1/upload/cf/upload" \
  -H "Authorization: Bearer <access_token>" \
  -H "Tus-Resumable: 1.0.0" \
  -H "Upload-Length: 123456789" \
  -H "Upload-Metadata: name dmlkZW8ubXA0"
```

**Success Response (200):**

```json
{
  "success": true,
  "message": "request was successful",
  "data": {
    "location": "https://upload.videodelivery.net/tus/abc123..."
  }
}
```

**Response Headers:**

- `Location` - The TUS upload URL for resumable uploads

**Error Responses:**

- `406 Not Acceptable` - Error from Cloudflare
- `413 Request Entity Too Large` - Out of storage

---

### Get Cloudflare Video Status 🔒

#### `GET /video/v1.1/upload/cf/{id}`

Get the processing status of a video in Cloudflare Stream.

**Authentication:** Required (Access Token)

**Path Parameters:**

| Parameter | Type   | Required | Description                     |
| --------- | ------ | -------- | ------------------------------- |
| `id`      | string | ✅       | The Cloudflare Stream video UID |

**Example Request:**

```bash
curl -X GET "https://api.example.com/video/v1.1/upload/cf/abc123" \
  -H "Authorization: Bearer <access_token>"
```

**Success Response (200):**

```json
{
  "success": true,
  "message": "Cloudflare status retrieved successfully",
  "data": {
    "result": {
      "uid": "abc123",
      "readyToStream": true,
      "status": {
        "state": "ready"
      },
      "thumbnail": "https://videodelivery.net/abc123/thumbnails/thumbnail.jpg",
      "playback": {
        "hls": "https://videodelivery.net/abc123/manifest/video.m3u8",
        "dash": "https://videodelivery.net/abc123/manifest/video.mpd"
      },
      "duration": 120,
      "input": {
        "width": 1920,
        "height": 1080
      }
    },
    "success": true
  }
}
```

---

### Update Cloudflare Video 🔒

#### `POST /video/v1.1/upload/cf/{id}`

Update or refresh a video's status in Cloudflare Stream.

**Authentication:** Required (Access Token)

**Path Parameters:**

| Parameter | Type   | Required | Description                     |
| --------- | ------ | -------- | ------------------------------- |
| `id`      | string | ✅       | The Cloudflare Stream video UID |

**Example Request:**

```bash
curl -X POST "https://api.example.com/video/v1.1/upload/cf/abc123" \
  -H "Authorization: Bearer <access_token>"
```

**Success Response (200):**

```json
{
  "success": true,
  "message": "Cloudflare update initiated successfully",
  "data": { ... }
}
```

---

### Generate Cloudflare Download 🔒

#### `POST /video/v1.1/upload/cf/{id}/generateDownload`

Generate a downloadable version of a Cloudflare Stream video.

**Authentication:** Required (Access Token)

**Path Parameters:**

| Parameter | Type   | Required | Description                     |
| --------- | ------ | -------- | ------------------------------- |
| `id`      | string | ✅       | The Cloudflare Stream video UID |

**Example Request:**

```bash
curl -X POST "https://api.example.com/video/v1.1/upload/cf/abc123/generateDownload" \
  -H "Authorization: Bearer <access_token>"
```

**Success Response (200):**

```json
{
  "success": true,
  "message": "Cloudflare download initiated successfully",
  "data": {
    "result": {
      "default": {
        "url": "https://videodelivery.net/abc123/downloads/default.mp4",
        "status": "ready"
      }
    }
  }
}
```

---

### Get Cloudflare Download Status 🔒

#### `GET /video/v1.1/upload/cf/{id}/download`

Check the status of a download generation request.

**Authentication:** Required (Access Token)

**Path Parameters:**

| Parameter | Type   | Required | Description                     |
| --------- | ------ | -------- | ------------------------------- |
| `id`      | string | ✅       | The Cloudflare Stream video UID |

**Response:**

- `501 Not Implemented` - This endpoint is currently not implemented

---

## Data Models

### Video

```json
{
  "id": 123,
  "uuid": "unique-identifier-string",
  "title": "Video Title",
  "description": "Video description text",
  "thumbnail": "https://content.hillview.tv/thumbnails/image.jpg",
  "cloudflare_id": "cloudflare-stream-uid",
  "url": "https://videodelivery.net/uid/manifest/video.m3u8",
  "download_url": "https://download-url.com/video.mp4",
  "allow_downloads": true,
  "views": 1234,
  "status": {
    "id": 1,
    "name": "Active",
    "short_name": "active"
  },
  "inserted_at": "2024-01-15T12:00:00Z"
}
```

### Playlist

```json
{
  "id": 1,
  "name": "Playlist Name",
  "description": "Playlist description",
  "banner_image": "https://content.hillview.tv/banners/image.jpg",
  "route": "playlist-url-slug",
  "inserted_at": "2024-01-15T12:00:00Z",
  "videos": [Video, ...]
}
```

### Spotlight

```json
{
  "rank": 1,
  "video_id": 123,
  "inserted_at": "2024-01-15T12:00:00Z",
  "updated_at": "2024-01-16T12:00:00Z",
  "video": Video
}
```

### Status (GeneralNSN)

```json
{
  "id": 1,
  "name": "Active",
  "short_name": "active"
}
```

---

## Rate Limiting

Currently, no rate limiting is implemented. Please be respectful of API usage.

---

## CORS

The API supports Cross-Origin Resource Sharing (CORS) with the following configuration:

- **Allowed Origins:** `*` (all origins)
- **Allowed Methods:** `GET`, `HEAD`, `POST`, `PUT`, `OPTIONS`
- **Allowed Headers:** `X-Requested-With`, `Content-Type`, `Origin`, `Authorization`, `Cookies`, `Accept`, `Cookie`, `X-CSRF-Token`, `Tus-Resumable`, `Upload-Length`, `Upload-Metadata`
- **Credentials:** Allowed

---

## Environment Variables

The API requires the following environment variables:

| Variable           | Description              |
| ------------------ | ------------------------ |
| `PORT`             | API server port          |
| `CLOUDFLARE_UID`   | Cloudflare account ID    |
| `CLOUDFLARE_EMAIL` | Cloudflare account email |
| `CLOUDFLARE_KEY`   | Cloudflare API key       |
| `CLOUDFLARE_TOKEN` | Cloudflare API token     |

---

## Changelog

### Version 1.1

- Base API version
- Video CRUD operations
- Playlist management
- Cloudflare Stream integration
- Newsletter subscription system
- Spotlight/featured content

### Version 2.1 Features (within v1.1 path)

- `/video/{query}` - Flexible video lookup by ID or UUID
- `/recordView/{query}` - View tracking
- `/recordDownload/{query}` - Download tracking

---

## Support

For API support or questions, please contact the HillviewTV development team.
