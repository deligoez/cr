package gh

import (
	"fmt"
	"strconv"
)

// Review is one review already on the pull request, as §8.4.4 reads it back.
//
// It carries the body and nothing about the comments under it. §8.4.4 matches
// the payload hash §8.4.3 embedded in the review's own body, so the comments
// decide nothing here — and asking for them would make the recovery read the
// cost of a whole round's threads to answer a question one string settles.
type Review struct {
	// ID is GitHub's node id for the review, which names the review a
	// reconciliation adopted.
	ID string `json:"id"`
	// URL is where a reader can open it, so the report names something
	// the reviewer can look at rather than an opaque id alone.
	URL string `json:"url"`
	// Body is the review's own body, which §8.4.3 embeds the payload hash
	// in.
	Body string `json:"body"`
}

// reviewPageSize is GitHub's maximum. A pull request whose reviews need a
// second page is exactly the one a reconciliation must still find its own
// review on, so the walk pages rather than truncating.
const reviewPageSize = 100

// reviewsQuery reads one page of the pull request's reviews.
//
// It is GraphQL for the reason pullRequestQuery is: the read boundary in
// write.go admits `gh api` and nothing else, and the document opens with
// `query`, which is what the boundary reads it by.
const reviewsQuery = `query($owner:String!,$repo:String!,$number:Int!,$reviews:Int!,$cursor:String){
  repository(owner:$owner,name:$repo){
    pullRequest(number:$number){
      reviews(first:$reviews,after:$cursor){
        pageInfo{hasNextPage endCursor}
        nodes{id url body}
      }
    }
  }
}`

// reviewsResponse is the answer's shape, in GitHub's own field names.
type reviewsResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				Reviews struct {
					PageInfo pageInfo `json:"pageInfo"`
					Nodes    []Review `json:"nodes"`
				} `json:"reviews"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// Reviews lists every review on one pull request, oldest first, which is
// §8.4.4's first clause.
//
// Nothing is filtered. §8.4.4 matches on the embedded hash, and a walk that
// dropped reviews by author or by state would decide for itself which of them
// could be cr's — on a pull request where a missed match means posting the
// same round twice, which §8.4.4 calls the worse failure.
func (c Client) Reviews(owner, repo string, number int) ([]Review, error) {
	reviews := make([]Review, 0)
	cursor := ""
	for {
		var page reviewsResponse
		args := []string{
			"api", "graphql",
			// -f rather than -F for the owner and the repository,
			// for the reason Threads gives: -F would convert a
			// name that looks like a number to the wrong GraphQL
			// type.
			"-f", "query=" + reviewsQuery,
			"-f", "owner=" + owner,
			"-f", "repo=" + repo,
			"-F", "number=" + strconv.Itoa(number),
			"-F", "reviews=" + strconv.Itoa(reviewPageSize),
		}
		if cursor != "" {
			args = append(args, "-f", "cursor="+cursor)
		}
		if err := c.query(&page, args); err != nil {
			return nil, err
		}
		connection := page.Data.Repository.PullRequest.Reviews
		reviews = append(reviews, connection.Nodes...)
		if !connection.PageInfo.HasNextPage {
			return reviews, nil
		}
		if connection.PageInfo.EndCursor == "" {
			return nil, fmt.Errorf(
				"%s/%s#%d: a page of reviews claims a next page and names no cursor",
				owner, repo, number,
			)
		}
		cursor = connection.PageInfo.EndCursor
	}
}
