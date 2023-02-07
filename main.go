package main

import (
	"bytes"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sts"
)

func UploadFile(s *session.Session, bucketName string, objectKey string, payload [][]string) error {
	body := ""

	for _, row := range payload {
		for i := 0; i < len(row); i++ {
			body = body + row[i]
			if i >= (len(row) - 1) {
				body = body + "\n"
			} else {
				body = body + ","
			}
		}
	}
	
	_, err := s3.New(s).PutObject(&s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
		Body:   bytes.NewReader([]byte(body)),
	})

	return err
}

func HandleRequest() {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	conf := aws.NewConfig().WithRegion("us-east-1")
	// s3session, err := session.NewSession(&aws.Config{
	//     Region: aws.String("us-east-2")},
	// )
	client := ec2.New(sess, conf)

	regions, err := client.DescribeRegions(&ec2.DescribeRegionsInput{})
	if err != nil {
		fmt.Println("Error", err)
	}

	var payload [][]string

	payload = append(payload, []string{"EC2Instance", "CPUUtilization", "AccountID", "Region", "TimeStamp"})

	var period int64
	period = 3600
	
	t := time.Now()
	rounded := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).UTC()

	for i:= 0; i < 10; i ++ {
		startTime := aws.Time(rounded.Add(time.Second * time.Duration(-3600 * (24 * (i + 1)))))
		endTime := aws.Time(rounded.Add(time.Second * time.Duration(-3600 * (1 + 24 * i ))))
		
		// fmt.Println("startTime", startTime)
		// fmt.Println("endTime", endTime)
		
		for _, region := range regions.Regions {
			// Create new EC2 client
			regionName := *region.RegionName
			// fmt.Println(regionName)

			regionConf := aws.NewConfig().WithRegion(regionName)
			client = ec2.New(sess, regionConf)
			cw := cloudwatch.New(sess, regionConf)
			stsConn := sts.New(sess, regionConf)
			if err != nil {
				fmt.Println("Error", err)
			}

			result, err := client.DescribeInstances(nil)

			if err != nil {
				fmt.Println("Error", err)
			}

			for _, r := range result.Reservations {
				for _, i := range r.Instances {
					search := cloudwatch.GetMetricStatisticsInput{
						StartTime:  startTime,
						EndTime:    endTime,
						MetricName: aws.String("CPUUtilization"),
						Period:     &period,
						Statistics: []*string{aws.String("Maximum")},
						Namespace:  aws.String("AWS/EC2"),
						Dimensions: []*cloudwatch.Dimension{{Name: aws.String("InstanceId"), Value: i.InstanceId}},
					}
					// fmt.Printf("InstanceID: %v State: %v\n", *i.InstanceId, i.State.Name)
					resp, err := cw.GetMetricStatistics(&search)
					
					if err != nil {
						fmt.Println("Error", err)
					}

					fmt.Println(resp)

					// fmt.Println(resp.Datapoints)
					outCallerIdentity, err := stsConn.GetCallerIdentity(&sts.GetCallerIdentityInput{})
					if err != nil {
						fmt.Println("Error", err)
					}
					accountID := *outCallerIdentity.Account
					// fmt.Println(accountID)

					for _, record := range resp.Datapoints {
						temp := strconv.FormatFloat(*record.Maximum, 'f', 6, 64)
						intTemp, _ := strconv.ParseInt(accountID, 10, 64)
						accountTemp := strconv.FormatInt(intTemp, 10)
						tempTime := record.Timestamp.String()
						payload = append(payload, []string{
							*i.InstanceId,
							temp,
							accountTemp,
							regionName,
							tempTime,
						})
					}
				}
			}
		}
		uploadFileName := fmt.Sprintf("CPUUtilization%02v.%02v.%v.csv", int(startTime.Month()), startTime.Day(), startTime.Year())
		err = UploadFile(sess, "monitoring-v0-test-new-files-per-day", uploadFileName, payload)
		if err != nil {
			fmt.Println("Error", err)
		}
		fmt.Println("Upload Finished")
	}
	
}

func main() {
	lambda.Start(HandleRequest)
}
