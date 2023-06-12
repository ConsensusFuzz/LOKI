/** @file GatewayConfig.h
 *  @author octopus
 *  @date 2021-05-19
 */

#pragma once
#include <bcos-gateway/Common.h>
#include <bcos-gateway/libnetwork/Common.h>
#include <boost/algorithm/string.hpp>
#include <boost/property_tree/ini_parser.hpp>
#include <boost/property_tree/ptree.hpp>

namespace bcos
{
namespace gateway
{
class GatewayConfig
{
public:
    using Ptr = std::shared_ptr<GatewayConfig>;

    GatewayConfig() = default;
    ~GatewayConfig() = default;

public:
    // cert for ssl connection
    struct CertConfig
    {
        std::string caCert;
        std::string nodeKey;
        std::string nodeCert;
    };

    // cert for sm ssl connection
    struct SMCertConfig
    {
        std::string caCert;
        std::string nodeCert;
        std::string nodeKey;
        std::string enNodeCert;
        std::string enNodeKey;
    };

public:
    /**
     * @brief: loads configuration items from the config.ini
     * @param _configPath: config.ini path
     * @return void
     */
    void initConfig(std::string const& _configPath);

    void setCertPath(std::string const& _certPath) { m_certPath = _certPath; }
    void setNodePath(std::string const& _nodePath) { m_nodePath = _nodePath; }
    void setNodeFileName(const std::string& _nodeFileName) { m_nodeFileName = _nodeFileName; }

    std::string const& certPath() const { return m_certPath; }
    std::string const& nodePath() const { return m_nodePath; }
    std::string const& nodeFileName() const { return m_nodeFileName; }

public:
    // check if the port valid
    bool isValidPort(int port);
    void hostAndPort2Endpoint(const std::string& _host, NodeIPEndpoint& _endpoint);
    void parseConnectedJson(const std::string& _json, std::set<NodeIPEndpoint>& _nodeIPEndpointSet);
    // loads p2p configuration items from the configuration file
    void initP2PConfig(const boost::property_tree::ptree& _pt);
    // loads ca configuration items from the configuration file
    void initCertConfig(const boost::property_tree::ptree& _pt);
    // loads sm ca configuration items from the configuration file
    void initSMCertConfig(const boost::property_tree::ptree& _pt);
    // check if file exist, exception will be throw if the file not exist
    void checkFileExist(const std::string& _path);
    // load p2p connected peers
    void loadP2pConnectedNodes();

public:
    std::string listenIP() const { return m_listenIP; }
    uint16_t listenPort() const { return m_listenPort; }
    uint32_t threadPoolSize() { return m_threadPoolSize; }
    bool smSSL() const { return m_smSSL; }

    CertConfig certConfig() const { return m_certConfig; }
    SMCertConfig smCertConfig() const { return m_smCertConfig; }
    const std::set<NodeIPEndpoint>& connectedNodes() const { return m_connectedNodes; }

private:
    // if SM SSL connection or not
    bool m_smSSL;
    // p2p network listen IP
    std::string m_listenIP;
    // p2p network listen Port
    uint16_t m_listenPort;
    // threadPool size
    uint32_t m_threadPoolSize{16};
    // p2p connected nodes host list
    std::set<NodeIPEndpoint> m_connectedNodes;
    // cert config for ssl connection
    CertConfig m_certConfig;
    SMCertConfig m_smCertConfig;

    std::string m_certPath;
    std::string m_nodePath;
    std::string m_nodeFileName;
};

}  // namespace gateway
}  // namespace bcos