package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

func (s *Server) profileAndRegion(r *http.Request) (string, string) {
	profile := r.PathValue("profile")
	region := r.URL.Query().Get("region")
	if region == "" {
		region = s.cfg.Region
	}
	return profile, region
}

func (s *Server) refresh(r *http.Request) bool {
	return r.URL.Query().Get("refresh") == "true"
}

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := awsinternal.ListProfiles()
	if err != nil {
		writeInternalError(w, "list profiles: "+err.Error())
		return
	}
	infos := make([]ProfileInfo, 0, len(profiles))
	for _, p := range profiles {
		info := ProfileInfo{
			Name:        p.Name,
			AccountID:   p.AccountID,
			SSORoleName: p.SSORoleName,
			Region:      p.Region,
			AuthType:    string(p.AuthType),
			SSOStatus:   string(p.SSOStatus),
		}
		// ゼロ値をそのまま Format すると "0001-01-01T00:00:00Z" が出力され
		// omitempty が効かないため、非ゼロのときだけ変換する。
		if !p.SSOExpiresAt.IsZero() {
			info.SSOExpiresAt = p.SSOExpiresAt.UTC().Format(time.RFC3339)
		}
		infos = append(infos, info)
	}
	writeJSON(w, infos)
}

// handleProfileIdentity resolves the AWS account ID for profile via STS
// GetCallerIdentity. Unlike handleListProfiles (static config parse), this
// makes a live AWS call and is invoked only for the profile the user
// selected, not for every profile in the list.
func (s *Server) handleProfileIdentity(w http.ResponseWriter, r *http.Request) {
	profile := r.PathValue("profile")
	if err := awsinternal.ValidateProfileName(profile); err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	identity, err := awsinternal.GetCallerIdentity(r.Context(), profile)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeJSON(w, CallerIdentityInfo{
		AccountID: identity.AccountID,
		Arn:       identity.ARN,
		UserID:    identity.UserID,
	})
}

func (s *Server) handleEC2(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("ec2", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListEC2Resources(r.Context(), profile, region)
	})
}

func (s *Server) handleRDS(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("rds", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListRDSResources(r.Context(), profile, region)
	})
}

func (s *Server) handleRDSParameters(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	group := r.URL.Query().Get("group")
	if group == "" {
		writeBadRequest(w, "group query parameter is required")
		return
	}
	s.serveCached(w, r, cacheKey("rds-parameters", profile, region, group), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListRDSParameters(r.Context(), profile, region, group)
	})
}

func (s *Server) handleRDSClusterParameters(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	cluster := r.URL.Query().Get("cluster")
	if cluster == "" {
		writeBadRequest(w, "cluster query parameter is required")
		return
	}
	s.serveCached(w, r, cacheKey("rds-cluster-parameters", profile, region, cluster), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListRDSClusterParameters(r.Context(), profile, region, cluster)
	})
}

func (s *Server) handleElastiCache(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("elasticache", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListElastiCacheResources(r.Context(), profile, region)
	})
}

func (s *Server) handleElastiCacheParameters(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	group := r.URL.Query().Get("group")
	if group == "" {
		writeBadRequest(w, "group query parameter is required")
		return
	}
	s.serveCached(w, r, cacheKey("elasticache-parameters", profile, region, group), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListElastiCacheParameters(r.Context(), profile, region, group)
	})
}

func (s *Server) handleLambda(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("lambda", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListLambdaResources(r.Context(), profile, region)
	})
}

func (s *Server) handleECS(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("ecs", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListECSResources(r.Context(), profile, region)
	})
}

func (s *Server) handleECSServices(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	cluster := r.PathValue("cluster")
	s.serveCached(w, r, cacheKey("ecs-services", profile, region, cluster), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListECSServices(r.Context(), profile, region, cluster)
	})
}

func (s *Server) handleECSTasks(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	cluster := r.PathValue("cluster")
	service := r.URL.Query().Get("service")
	s.serveCached(w, r, cacheKey("ecs-tasks", profile, region, cluster, service), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListECSTasks(r.Context(), profile, region, cluster, service)
	})
}

func (s *Server) handleECSContainers(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	cluster := r.PathValue("cluster")
	task := r.PathValue("task")
	s.serveCached(w, r, cacheKey("ecs-containers", profile, region, cluster, task), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListECSContainers(r.Context(), profile, region, cluster, task)
	})
}

func (s *Server) handleECR(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("ecr", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListECRResources(r.Context(), profile, region)
	})
}

func (s *Server) handleECRImages(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	repo := r.PathValue("repo")
	s.serveCached(w, r, cacheKey("ecr-images", profile, region, repo), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListECRImages(r.Context(), profile, region, repo)
	})
}

func (s *Server) handleS3(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("s3", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListS3Resources(r.Context(), profile, region)
	})
}

func (s *Server) handleIAM(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("iam", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListIAMResources(r.Context(), profile, region)
	})
}

func (s *Server) handleSSO(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("sso", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListSSOAccounts(r.Context(), profile, region)
	})
}

func (s *Server) handleSSMList(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("ssm-list", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListSSMParameters(r.Context(), profile, region)
	})
}

func (s *Server) handleSSMGet(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	name := r.PathValue("name")
	decrypt, _ := strconv.ParseBool(r.URL.Query().Get("decrypt"))
	// SSM parameter values are not cached (on-demand, decrypt flag varies).
	value, err := awsinternal.GetSSMParameter(r.Context(), profile, region, "/"+name, decrypt)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeJSON(w, SSMValueResponse{Value: value})
}

func (s *Server) handleSecretsList(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("secretsmanager-list", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListSecretResources(r.Context(), profile, region)
	})
}

func (s *Server) handleCFN(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("cfn", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListCFNStacks(r.Context(), profile, region)
	})
}

func (s *Server) handleCFNStackDetail(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	stack := r.PathValue("stack")
	s.serveCached(w, r, cacheKey("cfn-detail", profile, region, stack), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.DescribeCFNStackDetail(r.Context(), profile, region, stack)
	})
}

// cfnEventsCacheTTL はデプロイ進行中の確認が主用途のため既定の cacheTTL (1 時間) より短くする。
const cfnEventsCacheTTL = 30 * time.Second

func (s *Server) handleCFNStackEvents(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	stack := r.PathValue("stack")
	s.serveCached(w, r, cacheKey("cfn-events", profile, region, stack), cfnEventsCacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListCFNStackEvents(r.Context(), profile, region, stack)
	})
}

func (s *Server) handleCFNStackResources(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	stack := r.PathValue("stack")
	s.serveCached(w, r, cacheKey("cfn-resources", profile, region, stack), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListCFNStackResources(r.Context(), profile, region, stack)
	})
}

func (s *Server) handleKinesis(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("kinesis", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListKinesisResources(r.Context(), profile, region)
	})
}

func (s *Server) handleCloudFront(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("cloudfront", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListCloudFrontResources(r.Context(), profile, region)
	})
}

func (s *Server) handleCloudFrontInvalidation(w http.ResponseWriter, r *http.Request) {
	profile, _ := s.profileAndRegion(r)
	distID := r.PathValue("id")

	var body struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeBadRequest(w, "invalid request body: "+err.Error())
		return
	}
	if err := awsinternal.CreateCloudFrontInvalidation(r.Context(), profile, distID, body.Paths); err != nil {
		writeAWSError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleELB(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("elb", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListELBResources(r.Context(), profile, region)
	})
}

func (s *Server) handleELBListeners(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	lbArn := r.URL.Query().Get("lb_arn")
	if lbArn == "" {
		writeBadRequest(w, "lb_arn query parameter is required")
		return
	}
	s.serveCached(w, r, cacheKey("elb-listeners", profile, region, lbArn), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListELBListeners(r.Context(), profile, region, lbArn)
	})
}

func (s *Server) handleELBRules(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	listenerArn := r.URL.Query().Get("listener_arn")
	if listenerArn == "" {
		writeBadRequest(w, "listener_arn query parameter is required")
		return
	}
	s.serveCached(w, r, cacheKey("elb-rules", profile, region, listenerArn), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListELBRules(r.Context(), profile, region, listenerArn)
	})
}

func (s *Server) handleELBTargetGroups(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	lbArn := r.URL.Query().Get("lb_arn")
	if lbArn == "" {
		writeBadRequest(w, "lb_arn query parameter is required")
		return
	}
	s.serveCached(w, r, cacheKey("elb-target-groups", profile, region, lbArn), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListELBTargetGroups(r.Context(), profile, region, lbArn)
	})
}

func (s *Server) handleELBTargetHealth(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	tgArn := r.URL.Query().Get("tg_arn")
	if tgArn == "" {
		writeBadRequest(w, "tg_arn query parameter is required")
		return
	}
	s.serveCached(w, r, cacheKey("elb-target-health", profile, region, tgArn), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.DescribeELBTargetHealth(r.Context(), profile, region, tgArn)
	})
}

func (s *Server) handleDynamo(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("dynamo", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListDynamoResources(r.Context(), profile, region)
	})
}

func (s *Server) handleAPIGW(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("apigw", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListAPIGatewayResources(r.Context(), profile, region)
	})
}

func (s *Server) handleNATGW(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("natgw", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListNATGatewayResources(r.Context(), profile, region)
	})
}

func (s *Server) handleSQS(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("sqs", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListSQSResources(r.Context(), profile, region)
	})
}

func (s *Server) handleWAF(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	s.serveCached(w, r, cacheKey("waf", profile, region), cacheTTL, writeAWSError, func() (any, error) {
		return awsinternal.ListWAFResources(r.Context(), profile, region)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
