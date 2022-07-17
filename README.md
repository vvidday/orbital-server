# orbital-server
Backend server to handle storing/caching feature for tweets. Go webserver, PostgresQL database & deployed on Elastic Beanstalk*.

*Unfortunately, due to problems with the lack of https, the production is using the [main branch](https://github.com/vvidday/orbital-server/tree/main) that is deployed on Heroku. It is functionally the same, but swaps out [Echo](https://echo.labstack.com/) for [Gin](https://github.com/gin-gonic/gin) (due to issues with Heroku), and contains additional Heroku-specific configuration files. 

# Endpoints
The API exposes two GET endpoints, `/user` and `/tweets`:

## /user
Params:
```
username: string
```
Returns `user`  in JSON format:
```
{
    id: string
    name: string
    username: string
}
```

## /tweets
Params:
```
id: string
```
Returns `response`:
```
{
    data: tweet[50]
    meta: {
        result_count: int
        newest_id: string
        oldest_id: string
        next_token: string
    }
}
```
`tweet` object:
```
{
    id: string
    text: string
}
```

## Storage / Caching
Each request to the `tweets` endpoint is processed as follows:
- If tweets do not exist in the database, tweets are retrieved from the twitter API and stored in the database
- If tweets already exist in the database, the time they were retrieved is checked.
   - If the tweets were retrieved more than 12 hours ago, a fresh call to the twitter API is made, and the database entries are updated
   - Else, the existing tweets from the database are returned, avoiding the need for a call to the twitter API.

This thus enables a large reduction in calls made to the twitter API, while still maintaining the validity of tweets.

