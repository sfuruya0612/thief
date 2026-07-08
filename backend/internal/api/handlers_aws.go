package api

import (
	"encoding/json"
	"net/http"
	"strconv"

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
	var infos []ProfileInfo
	for _, p := range profiles {
		infos = append(infos, ProfileInfo{Name: p})
	}
	writeJSON(w, infos)
}

func (s *Server) handleEC2(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	refresh := s.refresh(r)
	key := cacheKey("ec2", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, refresh, func() (any, error) {
		return awsinternal.ListEC2Resources(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleRDS(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("rds", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListRDSResources(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleElastiCache(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("elasticache", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListElastiCacheResources(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleLambda(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("lambda", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListLambdaResources(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleECS(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("ecs", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListECSResources(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleECSServices(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	cluster := r.PathValue("cluster")
	key := cacheKey("ecs-services", profile, region, cluster)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListECSServices(r.Context(), profile, region, cluster)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleECSTasks(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	cluster := r.PathValue("cluster")
	service := r.URL.Query().Get("service")
	key := cacheKey("ecs-tasks", profile, region, cluster, service)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListECSTasks(r.Context(), profile, region, cluster, service)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleECSContainers(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	cluster := r.PathValue("cluster")
	task := r.PathValue("task")
	key := cacheKey("ecs-containers", profile, region, cluster, task)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListECSContainers(r.Context(), profile, region, cluster, task)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleECR(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("ecr", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListECRResources(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleECRImages(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	repo := r.PathValue("repo")
	key := cacheKey("ecr-images", profile, region, repo)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListECRImages(r.Context(), profile, region, repo)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleS3(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("s3", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListS3Resources(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleIAM(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("iam", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListIAMResources(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleSSO(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("sso", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListSSOAccounts(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleSSMList(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("ssm-list", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListSSMParameters(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
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
	key := cacheKey("secretsmanager-list", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListSecretResources(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleCFN(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("cfn", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListCFNStacks(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleKinesis(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("kinesis", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListKinesisResources(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleCloudFront(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("cloudfront", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListCloudFrontResources(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
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
	key := cacheKey("elb", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListELBResources(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleDynamo(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("dynamo", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListDynamoResources(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleAPIGW(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("apigw", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListAPIGatewayResources(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleNATGW(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("natgw", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListNATGatewayResources(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleSQS(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("sqs", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListSQSResources(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func (s *Server) handleWAF(w http.ResponseWriter, r *http.Request) {
	profile, region := s.profileAndRegion(r)
	key := cacheKey("waf", profile, region)
	entry, hit, err := s.resourceCache.Load(key, cacheTTL, s.refresh(r), func() (any, error) {
		return awsinternal.ListWAFResources(r.Context(), profile, region)
	})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeCacheHeaders(w, cacheHeadersFrom(hit, entry))
	writeJSON(w, entry.Value)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
