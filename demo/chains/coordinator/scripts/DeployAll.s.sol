/* solhint-disable no-console */
/* solhint-disable gas-small-strings */
// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import "forge-std/src/Script.sol";
import "forge-std/src/console2.sol";
import {Config} from "forge-std/src/Config.sol";

// === Core ===
import {IBCClient} from "@hyperledger-labs/yui-ibc-solidity/contracts/core/02-client/IBCClient.sol";
import {
    IBCConnectionSelfStateNoValidation as IBCConnection
} from "@hyperledger-labs/yui-ibc-solidity/contracts/core/03-connection/IBCConnectionSelfStateNoValidation.sol";
import {
    IBCChannelHandshake
} from "@hyperledger-labs/yui-ibc-solidity/contracts/core/04-channel/IBCChannelHandshake.sol";
import {
    IBCChannelPacketSendRecv
} from "@hyperledger-labs/yui-ibc-solidity/contracts/core/04-channel/IBCChannelPacketSendRecv.sol";
import {
    IBCChannelPacketTimeout
} from "@hyperledger-labs/yui-ibc-solidity/contracts/core/04-channel/IBCChannelPacketTimeout.sol";
import {
    IBCChannelUpgradeInitTryAck,
    IBCChannelUpgradeConfirmOpenTimeoutCancel
} from "@hyperledger-labs/yui-ibc-solidity/contracts/core/04-channel/IBCChannelUpgrade.sol";
import {
    OwnableIBCHandler as IBCHandler
} from "@hyperledger-labs/yui-ibc-solidity/contracts/core/25-handler/OwnableIBCHandler.sol";

// === App ===
import {CrossSimpleModule} from "@datachainlab/cross-solidity/src/core/CrossSimpleModule.sol";
import {TxAuthManager} from "@datachainlab/cross-solidity/src/core/TxAuthManager.sol";
import {TxManager} from "@datachainlab/cross-solidity/src/core/TxManager.sol";
import {MockClient} from "@hyperledger-labs/yui-ibc-solidity/contracts/clients/mock/MockClient.sol";
import {SampleExtensionVerifier} from "@datachainlab/cross-solidity/src/example/SampleExtensionVerifier.sol";
import {IAuthExtensionVerifier} from "@datachainlab/cross-solidity/src/core/IAuthExtensionVerifier.sol";
import {IIBCModuleInitializer} from "@hyperledger-labs/yui-ibc-solidity/contracts/core/26-router/IIBCModule.sol";
import {ILightClient} from "@hyperledger-labs/yui-ibc-solidity/contracts/core/02-client/ILightClient.sol";
import {IContractModule} from "@datachainlab/cross-solidity/src/core/IContractModule.sol";

// === User Contracts ===
import {MyERC20} from "src/MyERC20.sol";
import {MyERC20TransferModule} from "src/MyERC20TransferModule.sol";

contract DeployAll is Script, Config {
    // === Deployment artifacts (written back to deployments.toml) ===
    IBCHandler public ibcHandler;
    MyERC20 public myERC20;
    MyERC20TransferModule public transferModule;
    CrossSimpleModule public crossSimpleModule;
    TxManager public txManager;
    MockClient public mockClient;

    // ---------- helpers ----------
    function _deployCore() internal returns (IBCHandler) {
        console2.log("==> 01_DeployCore");
        IBCHandler handler = new IBCHandler(
            new IBCClient(),
            new IBCConnection(),
            new IBCChannelHandshake(),
            new IBCChannelPacketSendRecv(),
            new IBCChannelPacketTimeout(),
            new IBCChannelUpgradeInitTryAck(),
            new IBCChannelUpgradeConfirmOpenTimeoutCancel()
        );
        console2.log("  IBCHandler (Ownable):", address(handler));
        return handler;
    }

    function _deployApp(IBCHandler handler, bool debugMode)
        internal
        returns (MyERC20, MyERC20TransferModule, CrossSimpleModule, TxManager, MockClient)
    {
        console2.log("==> 02_DeployApp");

        // 1. Deploy Token
        MyERC20 token = new MyERC20();
        console2.log("  MyERC20:", address(token));

        // 2. Deploy Transfer Module (Not initialized yet)
        MyERC20TransferModule tModule = new MyERC20TransferModule();
        console2.log("  MyERC20TransferModule:", address(tModule));

        // 3. Prepare Verifiers
        SampleExtensionVerifier verifier = new SampleExtensionVerifier();
        console2.log("  SampleExtensionVerifier:", address(verifier));

        string[] memory typeUrls = new string[](1);
        typeUrls[0] = "/verifier.sample.extension";

        IAuthExtensionVerifier[] memory verifiers = new IAuthExtensionVerifier[](1);
        verifiers[0] = IAuthExtensionVerifier(verifier);

        // 4. Deploy Managers
        TxAuthManager txAuthManager = new TxAuthManager();
        console2.log("  TxAuthManager:", address(txAuthManager));

        TxManager tManager = new TxManager();
        console2.log("  TxManager:", address(tManager));

        // 5. Deploy CrossSimpleModule (Requires address of the App/Module)
        CrossSimpleModule cModule = new CrossSimpleModule(
            handler,
            address(txAuthManager),
            address(tManager),
            IContractModule(address(tModule)),
            typeUrls,
            verifiers,
            debugMode
        );
        console2.log("  CrossSimpleModule:", address(cModule));

        // 6. Deploy MockClient
        MockClient mclient = new MockClient(address(handler));
        console2.log("  MockClient:", address(mclient));

        return (token, tModule, cModule, tManager, mclient);
    }

    function _initialize(
        IBCHandler handler,
        CrossSimpleModule module,
        string memory portCross,
        string memory mockClientType,
        MockClient mclient,
        MyERC20TransferModule tModule,
        MyERC20 token
    ) internal {
        console2.log("==> 03_Initialize");

        // Initialize the Transfer Module
        tModule.initialize(address(module), address(token));
        console2.log("  -> MyERC20TransferModule Initialized with CrossModule and Token");

        handler.bindPort(portCross, IIBCModuleInitializer(module));
        handler.registerClient(mockClientType, ILightClient(mclient));
        console2.log("  Initialized. port=%s, clientType=%s", portCross, mockClientType);
    }

    function _readConfig()
        internal
        returns (
            string memory mnemonic,
            uint32 mnemonicIndex,
            bool debugMode,
            string memory portCross,
            string memory mockClientType
        )
    {
        string memory m = config.get("mnemonic").toString();
        uint256 idxU256 = config.get("mnemonic_index").toUint256();
        require(idxU256 < 2 ** 32, "mnemonic_index too large");
        uint32 idx = uint32(idxU256);
        bool dbg = config.get("debug_mode").toBool();
        string memory port = config.get("port_cross").toString();
        string memory cli = config.get("mock_client_type").toString();
        return (m, idx, dbg, port, cli);
    }

    function _logConfig(
        uint256 chainId,
        bool debugMode,
        string memory portCross,
        string memory mockClientType,
        uint32 mnemonicIndex
    ) internal {
        console2.log("Deploying to chain:", chainId);
        console2.log("Config:");
        console2.log("  debug_mode      :", debugMode);
        console2.log("  port_cross      :", portCross);
        console2.log("  mock_client_type:", mockClientType);
        console2.log("  mnemonic_index  :", mnemonicIndex);
    }

    function _deriveDeployer(string memory mnemonic, uint32 mnemonicIndex)
        internal
        returns (uint256 deployerPk, address deployer)
    {
        uint256 pk = vm.deriveKey(mnemonic, mnemonicIndex);
        address addr = vm.addr(pk);
        console2.log("Deployer:", addr);
        return (pk, addr);
    }

    function _broadcastDeployAndInit(
        uint256 deployerPk,
        bool debugMode,
        string memory portCross,
        string memory mockClientType
    ) internal {
        vm.startBroadcast(deployerPk);
        ibcHandler = _deployCore();
        (myERC20, transferModule, crossSimpleModule, txManager, mockClient) = _deployApp(ibcHandler, debugMode);
        _initialize(ibcHandler, crossSimpleModule, portCross, mockClientType, mockClient, transferModule, myERC20);
        vm.stopBroadcast();
    }

    function _writeBack(address deployer) internal {
        // save addresses & metadata to deployments.toml
        config.set("ibc_handler", address(ibcHandler));
        config.set("my_erc20", address(myERC20));
        config.set("my_erc20_transfer_module", address(transferModule));
        config.set("cross_simple_module", address(crossSimpleModule));
        config.set("tx_manager", address(txManager));
        config.set("mock_client", address(mockClient));

        // Meta
        config.set("deployer", deployer);
        console2.log("\nDeployment complete! Addresses saved to deployments.toml");
    }

    // ---------- entry ----------
    function run() external {
        _loadConfig("./deployments.toml", true);

        uint256 chainId = block.chainid;
        (
            string memory mnemonic,
            uint32 mnemonicIndex,
            bool debugMode,
            string memory portCross,
            string memory mockClientType
        ) = _readConfig();

        _logConfig(chainId, debugMode, portCross, mockClientType, mnemonicIndex);

        (uint256 deployerPk, address deployer) = _deriveDeployer(mnemonic, mnemonicIndex);

        _broadcastDeployAndInit(deployerPk, debugMode, portCross, mockClientType);

        _writeBack(deployer);
    }
}
