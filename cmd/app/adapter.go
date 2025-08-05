package app

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"

	protos "scow-crane-adapter/gen/go"
	"scow-crane-adapter/pkg/services/account"
	"scow-crane-adapter/pkg/services/app"
	"scow-crane-adapter/pkg/services/config"
	"scow-crane-adapter/pkg/services/job"
	"scow-crane-adapter/pkg/services/user"
	"scow-crane-adapter/pkg/services/version"
	"scow-crane-adapter/pkg/utils"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	FlagConfigFilePath string
	GConfig            utils.Config
)

func NewAdapterCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "scow-crane-adapter",
		Short:   "crane adapter for scow",
		Version: utils.GetVersion(),
		Run: func(cmd *cobra.Command, args []string) {
			Run()
		},
	}

	// Initialize config
	cobra.OnInitialize(func() {
		// Use config file from the flag or search in the default paths
		if FlagConfigFilePath != "" {
			viper.SetConfigFile(FlagConfigFilePath)
		} else {
			viper.AddConfigPath(".")
			viper.AddConfigPath("/etc/scow-crane-adapter/")
			viper.SetConfigType("yaml")
			viper.SetConfigName("config")
		}

		// Read and parse config file
		viper.ReadInConfig()
		// Initialize logger
		utils.InitLogger(utils.ParseLogLevel(viper.GetString("log-level")))
		if err := viper.Unmarshal(&GConfig); err != nil {
			logrus.Fatalf("Error parsing config file: %s", err)
		}

		logrus.Debugf("Using config:\n%+v", GConfig)
	})

	rootCmd.SetVersionTemplate(utils.VersionTemplate())
	// Specify config file path
	rootCmd.PersistentFlags().StringVarP(&FlagConfigFilePath, "config", "c", "", "Path to configuration file")

	// Other flags
	rootCmd.PersistentFlags().IntP("bind-port", "p", 5000, "Binding address of adapter")
	viper.BindPFlag("bind-addr", rootCmd.PersistentFlags().Lookup("addr"))

	rootCmd.PersistentFlags().StringP("log-level", "l", "info", "Log level")
	viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level"))

	return rootCmd
}

func Run() {
	// 初始化CraneCtld客户端及鹤思配置文件
	utils.InitClientAndCraneConfig()

	s := grpc.NewServer(
		grpc.MaxRecvMsgSize(1024*1024*1024), // 最大接受size 1GB
		grpc.MaxSendMsgSize(1024*1024*1024), // 最大发送size 1GB
		grpc.UnaryInterceptor(utils.UnaryServerLatencyInterceptor),
	)

	if GConfig.Ssl.Enabled {
		caCertPath, adapterCertPath, adapterPrivateKeyPath := GetCertPath()
		logrus.Tracef("caCertPath, adapterCertPath, adapterPrivateKeyPath: %s, %s, %s", caCertPath, adapterCertPath, adapterPrivateKeyPath)
		pair, err := tls.LoadX509KeyPair(adapterCertPath, adapterPrivateKeyPath)
		if err != nil {
			fmt.Println("LoadX509KeyPair error", err)
			return
		}
		// 创建一组根证书
		certPool := x509.NewCertPool()
		ca, err := ioutil.ReadFile(caCertPath)
		if err != nil {
			fmt.Println("read ca pem error ", err)
			return
		}
		// 解析证书
		if ok := certPool.AppendCertsFromPEM(ca); !ok {
			fmt.Println("AppendCertsFromPEM error ")
			return
		}

		cred := credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{pair},
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    certPool,
		})

		s = grpc.NewServer(
			grpc.MaxRecvMsgSize(1024*1024*1024), // 最大接受size 1GB
			grpc.MaxSendMsgSize(1024*1024*1024), // 最大发送size 1GB
			grpc.UnaryInterceptor(utils.UnaryServerLatencyInterceptor),
			grpc.Creds(cred),
		)
	}

	// 注册服务
	protos.RegisterJobServiceServer(s, &job.ServerJob{ModulePath: GConfig.ModulePath})
	protos.RegisterAccountServiceServer(s, &account.ServerAccount{})
	protos.RegisterConfigServiceServer(s, &config.ServerConfig{})
	protos.RegisterUserServiceServer(s, &user.ServerUser{})
	protos.RegisterVersionServiceServer(s, &version.ServerVersion{})
	protos.RegisterAppServiceServer(s, &app.ServerApp{})

	logrus.Infof("gRPC server listening on %d", GConfig.BindPort)
	portString := fmt.Sprintf(":%d", GConfig.BindPort)
	listener, err := net.Listen("tcp", portString)
	if err != nil {
		logrus.Fatalf("failed to listen: %s", err)
		return
	}

	if err := s.Serve(listener); err != nil {
		logrus.Fatalf("gRPC server quitting: %s", err)
	}
}
